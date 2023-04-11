// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otel // import "go.opentelemetry.io/otel"

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/metricembed"
	"go.opentelemetry.io/otel/metric/noop"
)

type testMeterProvider struct{ metricembed.MeterProvider }

var _ metric.MeterProvider = &testMeterProvider{}

func (*testMeterProvider) Meter(_ string, _ ...metric.MeterOption) metric.Meter {
	return noop.NewMeterProvider().Meter("")
}

func TestMultipleGlobalMeterProvider(t *testing.T) {
	p1 := testMeterProvider{}
	p2 := noop.NewMeterProvider()
	SetMeterProvider(&p1)
	SetMeterProvider(p2)

	got := GetMeterProvider()
	assert.Equal(t, p2, got)
}
