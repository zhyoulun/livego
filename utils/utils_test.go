package utils

import "testing"

func TestGenSessionID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "",
			want: "1",
		},
		{
			name: "",
			want: "2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenSessionIDString(); got != tt.want {
				t.Errorf("GenSessionID() = %v, want %v", got, tt.want)
			}
		})
	}
}
