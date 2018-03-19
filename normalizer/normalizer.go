package normalizer

import (
	"encoding/json"
	"fmt"

	"github.com/cisco/senml"
	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	nats "github.com/nats-io/go-nats"
)

const (
	queue   string = "normalizers"
	subject string = "src.*"
	output  string = "normalized"
)

type eventFlow struct {
	nc     *nats.Conn
	logger log.Logger
}

// Subscribe instantiates and starts a new NATS message flow.
func Subscribe(nc *nats.Conn, logger log.Logger) {
	flow := eventFlow{nc, logger}
	flow.start()
}

func (ef eventFlow) start() {
	ef.nc.QueueSubscribe(subject, queue, func(m *nats.Msg) {
		msg := mainflux.RawMessage{}

		if err := json.Unmarshal(m.Data, &msg); err != nil {
			ef.logger.Log("error", fmt.Sprintf("Unmarshalling failed: %s", err))
			return
		}

		if err := ef.publish(msg); err != nil {
			ef.logger.Log("error", fmt.Sprintf("Publishing failed: %s", err))
			return
		}
	})
}

func (ef eventFlow) publish(msg mainflux.RawMessage) error {
	normalized, err := ef.normalize(msg)
	if err != nil {
		ef.logger.Log("error", fmt.Sprintf("Normalization failed: %s", err))
		return err
	}

	for _, v := range normalized {
		data, err := json.Marshal(v)
		if err != nil {
			ef.logger.Log("error", fmt.Sprintf("Marshalling failed: %s", err))
			return err
		}

		if err = ef.nc.Publish(output, data); err != nil {
			ef.logger.Log("error", fmt.Sprintf("Publishing failed: %s", err))
			return err
		}
	}

	return nil
}

func (ef eventFlow) normalize(msg mainflux.RawMessage) ([]mainflux.Message, error) {
	var (
		raw, normalized senml.SenML
		err             error
	)

	if raw, err = senml.Decode(msg.Payload, senml.JSON); err != nil {
		return nil, err
	}

	normalized = senml.Normalize(raw)

	msgs := make([]mainflux.Message, len(normalized.Records))
	for k, v := range normalized.Records {
		m := mainflux.Message{
			Channel:     msg.Channel,
			Publisher:   msg.Publisher,
			Protocol:    msg.Protocol,
			Name:        v.Name,
			Unit:        v.Unit,
			StringValue: v.StringValue,
			DataValue:   v.DataValue,
			Time:        v.Time,
			UpdateTime:  v.UpdateTime,
			Link:        v.Link,
		}

		if v.Value != nil {
			m.Value = *v.Value
		}

		if v.BoolValue != nil {
			m.BoolValue = *v.BoolValue
		}

		if v.Sum != nil {
			m.ValueSum = *v.Sum
		}

		msgs[k] = m
	}

	return msgs, nil
}
