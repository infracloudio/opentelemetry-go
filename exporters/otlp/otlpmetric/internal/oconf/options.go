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

package oconf // import "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/internal/oconf"

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	"go.opentelemetry.io/otel/exporters/otlp/internal"
	"go.opentelemetry.io/otel/exporters/otlp/internal/retry"
	ominternal "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/internal"
	"go.opentelemetry.io/otel/internal/global"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
)

const (
	// DefaultMaxAttempts describes how many times the driver
	// should retry the sending of the payload in case of a
	// retryable error.
	DefaultMaxAttempts int = 5
	// DefaultMetricsPath is a default URL path for endpoint that
	// receives metrics.
	DefaultMetricsPath string = "/v1/metrics"
	// DefaultBackoff is a default base backoff time used in the
	// exponential backoff strategy.
	DefaultBackoff time.Duration = 300 * time.Millisecond
	// DefaultTimeout is a default max waiting time for the backend to process
	// each span or metrics batch.
	DefaultTimeout time.Duration = 10 * time.Second
)

type (
	SignalConfig struct {
		Endpoint    string
		Insecure    bool
		TLSCfg      *tls.Config
		Headers     map[string]string
		Compression Compression
		Timeout     time.Duration
		URLPath     string

		// gRPC configurations
		GRPCCredentials credentials.TransportCredentials

		TemporalitySelector metric.TemporalitySelector
		AggregationSelector metric.AggregationSelector
	}

	Config struct {
		// Signal specific configurations
		Metrics SignalConfig

		RetryConfig retry.Config

		// gRPC configurations
		ReconnectionPeriod time.Duration
		ServiceConfig      string
		DialOptions        []grpc.DialOption
		GRPCConn           *grpc.ClientConn
	}
)

// NewHTTPConfig returns a new Config with all settings applied from opts and
// any unset setting using the default HTTP config values.
func NewHTTPConfig(opts ...HTTPOption) Config {
	cfg := Config{
		Metrics: SignalConfig{
			Endpoint:    fmt.Sprintf("%s:%d", DefaultCollectorHost, DefaultCollectorHTTPPort),
			URLPath:     DefaultMetricsPath,
			Compression: NoCompression,
			Timeout:     DefaultTimeout,

			TemporalitySelector: metric.DefaultTemporalitySelector,
			AggregationSelector: metric.DefaultAggregationSelector,
		},
		RetryConfig: retry.DefaultConfig,
	}
	cfg = ApplyHTTPEnvConfigs(cfg)
	for _, opt := range opts {
		cfg = opt.ApplyHTTPOption(cfg)
	}
	cfg.Metrics.URLPath = internal.CleanPath(cfg.Metrics.URLPath, DefaultMetricsPath)
	return cfg
}

// NewGRPCConfig returns a new Config with all settings applied from opts and
// any unset setting using the default gRPC config values.
func NewGRPCConfig(opts ...GRPCOption) Config {
	cfg := Config{
		Metrics: SignalConfig{
			Endpoint:    fmt.Sprintf("%s:%d", DefaultCollectorHost, DefaultCollectorGRPCPort),
			URLPath:     DefaultMetricsPath,
			Compression: NoCompression,
			Timeout:     DefaultTimeout,

			TemporalitySelector: metric.DefaultTemporalitySelector,
			AggregationSelector: metric.DefaultAggregationSelector,
		},
		RetryConfig: retry.DefaultConfig,
		DialOptions: []grpc.DialOption{grpc.WithUserAgent(ominternal.GetUserAgentHeader())},
	}
	cfg = ApplyGRPCEnvConfigs(cfg)
	for _, opt := range opts {
		cfg = opt.ApplyGRPCOption(cfg)
	}

	if cfg.ServiceConfig != "" {
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithDefaultServiceConfig(cfg.ServiceConfig))
	}
	// Priroritize GRPCCredentials over Insecure (passing both is an error).
	if cfg.Metrics.GRPCCredentials != nil {
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithTransportCredentials(cfg.Metrics.GRPCCredentials))
	} else if cfg.Metrics.Insecure {
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// Default to using the host's root CA.
		creds := credentials.NewTLS(nil)
		cfg.Metrics.GRPCCredentials = creds
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithTransportCredentials(creds))
	}
	if cfg.Metrics.Compression == GzipCompression {
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)))
	}
	if len(cfg.DialOptions) != 0 {
		cfg.DialOptions = append(cfg.DialOptions, cfg.DialOptions...)
	}
	if cfg.ReconnectionPeriod != 0 {
		p := grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: cfg.ReconnectionPeriod,
		}
		cfg.DialOptions = append(cfg.DialOptions, grpc.WithConnectParams(p))
	}

	return cfg
}

type (
	// GenericOption applies an option to the HTTP or gRPC driver.
	GenericOption interface {
		ApplyHTTPOption(Config) Config
		ApplyGRPCOption(Config) Config

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// HTTPOption applies an option to the HTTP driver.
	HTTPOption interface {
		ApplyHTTPOption(Config) Config

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// GRPCOption applies an option to the gRPC driver.
	GRPCOption interface {
		ApplyGRPCOption(Config) Config

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}
)

// genericOption is an option that applies the same logic
// for both gRPC and HTTP.
type genericOption struct {
	fn func(Config) Config
}

func (g *genericOption) ApplyGRPCOption(cfg Config) Config {
	return g.fn(cfg)
}

func (g *genericOption) ApplyHTTPOption(cfg Config) Config {
	return g.fn(cfg)
}

func (genericOption) private() {}

func newGenericOption(fn func(cfg Config) Config) GenericOption {
	return &genericOption{fn: fn}
}

// splitOption is an option that applies different logics
// for gRPC and HTTP.
type splitOption struct {
	httpFn func(Config) Config
	grpcFn func(Config) Config
}

func (g *splitOption) ApplyGRPCOption(cfg Config) Config {
	return g.grpcFn(cfg)
}

func (g *splitOption) ApplyHTTPOption(cfg Config) Config {
	return g.httpFn(cfg)
}

func (splitOption) private() {}

func newSplitOption(httpFn func(cfg Config) Config, grpcFn func(cfg Config) Config) GenericOption {
	return &splitOption{httpFn: httpFn, grpcFn: grpcFn}
}

// httpOption is an option that is only applied to the HTTP driver.
type httpOption struct {
	fn func(Config) Config
}

func (h *httpOption) ApplyHTTPOption(cfg Config) Config {
	return h.fn(cfg)
}

func (httpOption) private() {}

func NewHTTPOption(fn func(cfg Config) Config) HTTPOption {
	return &httpOption{fn: fn}
}

// grpcOption is an option that is only applied to the gRPC driver.
type grpcOption struct {
	fn func(Config) Config
}

func (h *grpcOption) ApplyGRPCOption(cfg Config) Config {
	return h.fn(cfg)
}

func (grpcOption) private() {}

func NewGRPCOption(fn func(cfg Config) Config) GRPCOption {
	return &grpcOption{fn: fn}
}

// Generic Options

func WithEndpoint(endpoint string) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		// Add scheme if not present
		if !internal.HasScheme(endpoint) {
			endpoint = getScheme(cfg) + "://" + endpoint
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			global.Error(err, "parse url", "input", endpoint)
			return cfg
		}
		cfg.Metrics.Endpoint = u.Host
		// For OTLP/HTTP endpoint URLs without a per-signal
		// configuration, the passed endpoint is used as a base URL
		// and the signals are sent to these paths relative to that.
		cfg.Metrics.URLPath = path.Join(u.Path, DefaultMetricsPath)
		return cfg
	})
}

// getScheme fetches scheme based on config.
func getScheme(cfg Config) string {
	if cfg.Metrics.Insecure {
		return "http"
	}
	return "https"
}

func WithCompression(compression Compression) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.Compression = compression
		return cfg
	})
}

func WithURLPath(urlPath string) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.URLPath = urlPath
		return cfg
	})
}

func WithRetry(rc retry.Config) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.RetryConfig = rc
		return cfg
	})
}

func WithTLSClientConfig(tlsCfg *tls.Config) GenericOption {
	return newSplitOption(func(cfg Config) Config {
		cfg.Metrics.TLSCfg = tlsCfg.Clone()
		return cfg
	}, func(cfg Config) Config {
		cfg.Metrics.GRPCCredentials = credentials.NewTLS(tlsCfg)
		return cfg
	})
}

func WithInsecure() GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.Insecure = true
		return cfg
	})
}

func WithSecure() GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.Insecure = false
		return cfg
	})
}

func WithHeaders(headers map[string]string) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.Headers = headers
		return cfg
	})
}

func WithTimeout(duration time.Duration) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.Timeout = duration
		return cfg
	})
}

func WithTemporalitySelector(selector metric.TemporalitySelector) GenericOption {
	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.TemporalitySelector = selector
		return cfg
	})
}

func WithAggregationSelector(selector metric.AggregationSelector) GenericOption {
	// Deep copy and validate before using.
	wrapped := func(ik metric.InstrumentKind) aggregation.Aggregation {
		a := selector(ik)
		cpA := a.Copy()
		if err := cpA.Err(); err != nil {
			cpA = metric.DefaultAggregationSelector(ik)
			global.Error(
				err, "using default aggregation instead",
				"aggregation", a,
				"replacement", cpA,
			)
		}
		return cpA
	}

	return newGenericOption(func(cfg Config) Config {
		cfg.Metrics.AggregationSelector = wrapped
		return cfg
	})
}
