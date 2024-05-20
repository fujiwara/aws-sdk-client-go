package sdkclient

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/itchyny/gojq"
)

type clientMethodParam struct {
	InputBytes        json.RawMessage
	InputReader       io.Reader
	InputReaderLength *int64

	awsCfg  aws.Config
	cleanup []func() error
}

func (p *clientMethodParam) Cleanup() {
	for _, f := range p.cleanup {
		if err := f(); err != nil {
			log.Printf("[warn] failed to cleanup: %v", err)
		}
	}
}

func (p *clientMethodParam) mustInject(field string, value *int64) {
	v := make(map[string]any)
	if err := json.Unmarshal(p.InputBytes, &v); err != nil {
		panic(fmt.Sprintf("failed to marshal %s:", err))
	}
	var q string
	if value == nil {
		q = fmt.Sprintf("del(.%s)", field)
	} else {
		q = fmt.Sprintf(".%s = %d", field, *value)
	}
	query, err := gojq.Parse(q)
	if err != nil {
		panic(fmt.Sprintf("failed to parse query %s: %v", q, err))
	}
	iter := query.Run(v)
	for {
		if v, ok := iter.Next(); ok {
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
					break
				}
				panic(err)
			}
			p.InputBytes, _ = json.Marshal(v)
			return
		}
	}
}
