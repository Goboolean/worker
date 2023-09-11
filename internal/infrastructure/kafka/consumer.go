package kafka

import (
	"context"
	"sync"
	"time"

	"github.com/Goboolean/common/pkg/resolver"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde/protobuf"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)



type Deserializer interface {
	DeserializeInto(topic string, payload []byte, msg interface{}) error
}

type ProtoDeserializer struct {}

func (s *ProtoDeserializer) DeserializeInto(topic string, payload []byte, msg interface{}) error {
	return proto.Unmarshal(payload, msg.(proto.Message))
}

func newDeserializer() Deserializer {
	return &ProtoDeserializer{}
}


type SubscribeListener[T proto.Message] interface {
	OnReceiveMessage(ctx context.Context, msg T) error
}


type Consumer[T proto.Message] struct {
	consumer *kafka.Consumer
	deserial Deserializer

	listener SubscribeListener[T]
	topic string
	channel chan T

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// example:
// p, err := NewConsumer(&resolver.ConfigMap{
//   "BOOTSTRAP_HOST": os.Getenv("KAFKA_BOOTSTRAP_HOST"),
//   "REGISTRY_HOST":  os.Getenv("KAFKA_REGISTRY_HOST"), // optional
//   "GROUP_ID":       "GROUP_ID",
// }, subscribeListenerImpl)
func NewConsumer[T proto.Message](c *resolver.ConfigMap, l SubscribeListener[T]) (*Consumer[T], error) {

	bootstrap_host, err := c.GetStringKey("BOOTSTRAP_HOST")
	if err != nil {
		return nil, err
	}

	group_id, err := c.GetStringKey("GROUP_ID")
	if err != nil {
		return nil, err
	}

	conn, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":   bootstrap_host,
		"group.id":            group_id,
	})

	ctx, cancel := context.WithCancel(context.Background())

	instance := &Consumer[T]{
		consumer: conn,
		listener: l,
		wg: sync.WaitGroup{},
		ctx: ctx,
		cancel: cancel,
	}

	registry_url, exists, err := c.GetStringKeyOptional("REGISTRY_URL")
	if err != nil {
		return nil, err
	}

	if exists {
		sr, err := schemaregistry.NewClient(schemaregistry.NewConfig(registry_url))
		if err != nil {
			return nil, err
		}

		d, err := protobuf.NewDeserializer(sr, serde.ValueSerde, protobuf.NewDeserializerConfig())
		if err != nil {
			return nil, err
		}

		instance.deserial = d
	} else {
		instance.deserial = newDeserializer()
	}

	return instance, nil
}


func (c *Consumer[T]) Subscribe(topic string, schema protoreflect.MessageType) error {

	_, ok := c.deserial.(*protobuf.Deserializer)
	if ok {
		if err := c.deserial.(*protobuf.Deserializer).ProtoRegistry.RegisterMessage(schema); err != nil {
			return err
		}
	}

	if err := c.consumer.SubscribeTopics([]string{topic}, nil); err != nil {
		return err
	}
	c.topic = topic
	return nil
}


func (c *Consumer[T]) readMessage() {
	go func() {
		c.wg.Add(1)
		defer c.wg.Done()

		for {
			if err := c.ctx.Err(); err != nil {
				return
			}

			msg, err := c.consumer.ReadMessage(time.Second * 1)
			if err != nil {
				continue
			}

			var event T
			if err := c.deserial.DeserializeInto(c.topic, msg.Value, event); err != nil {
				log.WithFields(log.Fields{
					"topic": *msg.TopicPartition.Topic,
					"data":  msg.Value,
					"error": err,
				}).Error("Failed to deserialize data")
				continue
			}

			c.channel <- event
		}
	}()
}


func (c *Consumer[T]) consumeMessage(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-c.channel:
			ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
			if err := c.listener.OnReceiveMessage(ctx, event); err != nil {
				log.WithFields(log.Fields{
					"event":  event,
					"error": err,
				}).Error("Failed to process data")
			}
			cancel()
		}
	}
}


func (c *Consumer[T]) Close() {
	c.consumer.Close()
	c.cancel()
	c.wg.Wait()
}


func (c *Consumer[T]) Ping(ctx context.Context) error {
	// It requires ctx to be deadline set, otherwise it will return error
	// It will return error if there is no response within deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		return ErrDeadlineSettingRequired
	}

	remaining := time.Until(deadline)
	_, err := c.consumer.GetMetadata(nil, true, int(remaining.Milliseconds()))
	return err
}