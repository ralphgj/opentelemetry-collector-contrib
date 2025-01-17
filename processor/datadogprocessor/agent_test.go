// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datadogprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/datadogprocessor"

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/otlp/model/translator"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/require"
)

func TestTraceAgentConfig(t *testing.T) {
	cfg := traceconfig.New()
	require.NotZero(t, cfg.ReceiverPort)

	out := make(chan pb.StatsPayload)
	agnt := newAgentWithConfig(context.Background(), cfg, out)
	require.Zero(t, cfg.ReceiverPort)
	require.NotEmpty(t, cfg.Endpoints[0].APIKey)
	require.Equal(t, translator.UnsetHostnamePlaceholder, cfg.Hostname)
	require.Equal(t, out, agnt.Concentrator.Out)
}

func TestTraceAgent(t *testing.T) {
	cfg := traceconfig.New()
	cfg.BucketInterval = 50 * time.Millisecond
	out := make(chan pb.StatsPayload, 10)
	ctx := context.Background()
	a := newAgentWithConfig(ctx, cfg, out)
	a.Start()
	defer a.Stop()

	rspanss := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "1"},
				{Name: "2"},
				{Name: "3"},
			},
		},
		{
			LibName:    "other-libname",
			LibVersion: "2.1",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "4", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
				{Name: "5", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}},
			},
		},
	}).Traces()

	a.Ingest(ctx, rspanss)
	var stats pb.StatsPayload
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case stats = <-out:
			if len(stats.Stats) != 0 {
				break loop
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
	require.Len(t, stats.Stats, 1)
	require.Len(t, stats.Stats[0].Stats, 1)
	// considering all spans in rspans have distinct aggregations, we should have an equal amount
	// of groups
	require.Len(t, stats.Stats[0].Stats[0].Stats, rspanss.SpanCount())
	require.Len(t, a.TraceWriter.In, 0) // the trace writer channel should've been drained
}
