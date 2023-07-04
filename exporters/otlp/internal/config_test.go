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

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanPath(t *testing.T) {
	type args struct {
		urlPath     string
		defaultPath string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "clean empty path",
			args: args{
				urlPath:     "",
				defaultPath: "DefaultPath",
			},
			want: "DefaultPath",
		},
		{
			name: "clean metrics path",
			args: args{
				urlPath:     "/prefix/v1/metrics",
				defaultPath: "DefaultMetricsPath",
			},
			want: "/prefix/v1/metrics",
		},
		{
			name: "clean traces path",
			args: args{
				urlPath:     "https://env_endpoint",
				defaultPath: "DefaultTracesPath",
			},
			want: "/https:/env_endpoint",
		},
		{
			name: "spaces trimmed",
			args: args{
				urlPath: " /dir",
			},
			want: "/dir",
		},
		{
			name: "clean path empty",
			args: args{
				urlPath:     "dir/..",
				defaultPath: "DefaultTracesPath",
			},
			want: "DefaultTracesPath",
		},
		{
			name: "make absolute",
			args: args{
				urlPath: "dir/a",
			},
			want: "/dir/a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanPath(tt.args.urlPath, tt.args.defaultPath); got != tt.want {
				t.Errorf("CleanPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasScheme(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "No scheme",
			url:      "localhost:3422",
			expected: false,
		},
		{
			name:     "With secure scheme",
			url:      "https://127.0.0.1:3422",
			expected: true,
		},
		{
			name:     "With upper case secure scheme",
			url:      "HtTpS://127.0.0.1:3422",
			expected: true,
		},
		{
			name:     "With insecure scheme",
			url:      "http://127.0.0.1:3422",
			expected: true,
		},
		{
			name:     "With upper case insecure scheme",
			url:      "HtTp://127.0.0.1:3422",
			expected: true,
		},
		{
			name:     "With invalid scheme",
			url:      "ftp://127.0.0.1:3422",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasScheme(tt.url))
		})
	}
}
