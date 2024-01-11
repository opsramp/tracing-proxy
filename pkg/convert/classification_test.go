package convert

import (
	"reflect"
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func TestNormalizeClassification(t *testing.T) {
	type args struct {
		m    map[string]string
		args [][]*commonpb.KeyValue
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "nil map & args",
			args: args{
				m:    nil,
				args: nil,
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "unknown",
				"transaction.sub_category": "unknown",
				"language":                 "unknown",
			},
		},
		{
			name: "use old language",
			args: args{
				m: map[string]string{
					"language": "Python",
				},
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "db.system",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "mysql"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "Databases",
				"transaction.sub_category": "mysql",
				"language":                 "Python",
			},
		},
		{
			name: "already set as web",
			args: args{
				m: map[string]string{
					"transaction.type":         "web",
					"transaction.category":     "Programming Language",
					"transaction.sub_category": "go",
					"language":                 "go",
				},
				args: [][]*commonpb.KeyValue{},
			},
			want: map[string]string{
				"transaction.type":         "web",
				"transaction.category":     "Programming Language",
				"transaction.sub_category": "go",
				"language":                 "go",
			},
		},
		{
			name: "RPC Classification",
			args: args{
				m: map[string]string{
					"transaction.type":         "non-web",
					"transaction.category":     "Programming Language",
					"transaction.sub_category": "go",
					"language":                 "go",
				},
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "rpc.system",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "GRPC"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "web",
				"transaction.category":     "RPC Systems",
				"transaction.sub_category": "GRPC",
				"language":                 "go",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeClassification(tt.args.m, tt.args.args...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NormalizeClassification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineClassification(t *testing.T) {
	type args struct {
		args [][]*commonpb.KeyValue
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "Programming Language Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "telemetry.sdk.language",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "go"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "Programming Language",
				"transaction.sub_category": "go",
				"language":                 "go",
			},
		},
		{
			name: "Exceptions Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "exception.type",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "OutOfBounds"},
							},
						},
					},
					{
						{
							Key: "telemetry.sdk.language",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "java"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "Exceptions",
				"transaction.sub_category": "OutOfBounds",
				"language":                 "java",
			},
		},
		{
			name: "RPC Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "rpc.system",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "GRPC"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "web",
				"transaction.category":     "RPC Systems",
				"transaction.sub_category": "GRPC",
				"language":                 "unknown",
			},
		},
		{
			name: "messaging Queues Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "messaging.system",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "kafka"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "Messaging queues",
				"transaction.sub_category": "kafka",
				"language":                 "unknown",
			},
		},
		{
			name: "HTTP Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "http.request.method",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "test"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "web",
				"transaction.category":     "HTTP",
				"transaction.sub_category": "test",
				"language":                 "unknown",
			},
		},
		{
			name: "DB Classification",
			args: args{
				args: [][]*commonpb.KeyValue{
					{
						{
							Key: "db.system",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "mysql"},
							},
						},
						{
							Key: "telemetry.sdk.language",
							Value: &commonpb.AnyValue{
								Value: &commonpb.AnyValue_StringValue{StringValue: "c"},
							},
						},
					},
				},
			},
			want: map[string]string{
				"transaction.type":         "non-web",
				"transaction.category":     "Databases",
				"transaction.sub_category": "mysql",
				"language":                 "c",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetermineClassification(tt.args.args...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DetermineClassification() = %v, want %v", got, tt.want)
			}
		})
	}
}
