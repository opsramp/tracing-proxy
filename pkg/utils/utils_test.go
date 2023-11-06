package utils

import (
	"reflect"
	"testing"
)

func TestSyncedMap_Clear(t *testing.T) {
	type testCase[K comparable, V any] struct {
		name string
		m    SyncedMap[K, V]
	}
	tests := []testCase[string, string]{
		{
			name: "clear",
			m: SyncedMap[string, string]{
				m: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Clear()
			if len(tt.m.m) != 0 {
				t.Errorf("failed to clear")
			}
		})
	}
}

func TestSyncedMap_Delete(t *testing.T) {
	type args[K comparable] struct {
		key K
	}
	type testCase[K comparable, V any] struct {
		name string
		m    SyncedMap[K, V]
		args args[K]
		want map[K]V
	}
	tests := []testCase[string, string]{
		{
			name: "delete key 'test'",
			m: SyncedMap[string, string]{
				m: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			args: args[string]{
				key: "key1",
			},
			want: map[string]string{
				"key2": "value2",
			},
		},
		{
			name: "delete key 'test'",
			m: SyncedMap[string, string]{
				m: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			args: args[string]{
				key: "key2",
			},
			want: map[string]string{
				"key1": "value1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Delete(tt.args.key)
			if !reflect.DeepEqual(tt.m.m, tt.want) {
				t.Errorf("failed %v", tt.name)
			}
		})
	}
}

func TestSyncedMap_Get(t *testing.T) {
	type args[K comparable] struct {
		key K
	}
	type testCase[K comparable, V any] struct {
		name string
		m    SyncedMap[K, V]
		args args[K]
		want V
	}
	tests := []testCase[string, string]{
		{
			name: "get key1",
			m: SyncedMap[string, string]{
				m: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			args: args[string]{
				key: "key1",
			},
			want: "value1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.Get(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncedMap_Set(t *testing.T) {
	type args[K comparable, V any] struct {
		key   K
		value V
	}
	type testCase[K comparable, V any] struct {
		name string
		m    SyncedMap[K, V]
		args args[K, V]
		want map[K]V
	}
	tests := []testCase[string, string]{
		{
			name: "add key1",
			m: SyncedMap[string, string]{
				m: map[string]string{},
			},
			args: args[string, string]{
				key:   "key1",
				value: "value1",
			},
			want: map[string]string{
				"key1": "value1",
			},
		},
		{
			name: "add empty key",
			m: SyncedMap[string, string]{
				m: map[string]string{},
			},
			args: args[string, string]{
				key:   "",
				value: "value1",
			},
			want: map[string]string{
				"": "value1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.Set(tt.args.key, tt.args.value)
		})
	}
}
