// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package probabilisticsamplerprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"
)

func TestNewLogsProcessor(t *testing.T) {
	tests := []struct {
		name         string
		nextConsumer consumer.Logs
		cfg          *Config
		wantErr      bool
	}{
		{
			name: "nil_nextConsumer",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 15.5,
			},
			wantErr: true,
		},
		{
			name:         "happy_path",
			nextConsumer: consumertest.NewNop(),
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 15.5,
			},
		},
		{
			name:         "happy_path_hash_seed",
			nextConsumer: consumertest.NewNop(),
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 13.33,
				HashSeed:           4321,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newLogsProcessor(context.Background(), processortest.NewNopCreateSettings(), tt.nextConsumer, tt.cfg)
			if tt.wantErr {
				assert.Nil(t, got)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestLogsSampling(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		received int
	}{
		{
			name: "happy_path",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 100,
			},
			received: 100,
		},
		{
			name: "nothing",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 0,
			},
			received: 0,
		},
		{
			name: "roughly half",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 50,
				AttributeSource:    traceIDAttributeSource,
			},
			received: 52,
		},
		{
			name: "sampling_source no sampling",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 0,
				AttributeSource:    recordAttributeSource,
				FromAttribute:      "foo",
			},
			received: 0,
		},
		{
			name: "sampling_source all sampling",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 100,
				AttributeSource:    recordAttributeSource,
				FromAttribute:      "foo",
			},
			received: 100,
		},
		{
			name: "sampling_source sampling",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 50,
				AttributeSource:    recordAttributeSource,
				FromAttribute:      "foo",
			},
			received: 79,
		},
		{
			name: "sampling_priority",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 0,
				SamplingPriority:   "priority",
			},
			received: 25,
		},
		{
			name: "sampling_priority with sampling field",
			cfg: &Config{
				ProcessorSettings:  config.NewProcessorSettings(component.NewID(typeStr)),
				SamplingPercentage: 0,
				AttributeSource:    recordAttributeSource,
				FromAttribute:      "foo",
				SamplingPriority:   "priority",
			},
			received: 25,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := new(consumertest.LogsSink)
			processor, err := newLogsProcessor(context.Background(), processortest.NewNopCreateSettings(), sink, tt.cfg)
			require.NoError(t, err)
			logs := plog.NewLogs()
			lr := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
			for i := 0; i < 100; i++ {
				record := lr.AppendEmpty()
				record.SetTimestamp(pcommon.Timestamp(time.Unix(1649400860, 0).Unix()))
				record.SetSeverityNumber(plog.SeverityNumberDebug)
				ib := byte(i)
				traceID := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, ib, ib, ib, ib, ib, ib, ib, ib}
				record.SetTraceID(traceID)
				// set half of records with a foo attribute
				if i%2 == 0 {
					b := record.Attributes().PutEmptyBytes("foo")
					b.FromRaw(traceID[:])
				}
				// set a fourth of records with a priority attribute
				if i%4 == 0 {
					record.Attributes().PutDouble("priority", 100)
				}
			}
			err = processor.ConsumeLogs(context.Background(), logs)
			require.NoError(t, err)
			sunk := sink.AllLogs()
			numReceived := 0
			if len(sunk) > 0 && sunk[0].ResourceLogs().Len() > 0 {
				numReceived = sunk[0].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().Len()
			}
			assert.Equal(t, tt.received, numReceived)
		})
	}
}
