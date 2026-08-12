package email

import (
	"net"
	"testing"
)

func TestUnsafeAttachmentIPClassification(t *testing.T) {
	tests := []struct {
		raw    string
		unsafe bool
	}{
		{"127.0.0.1", true}, {"10.1.2.3", true}, {"172.16.0.1", true}, {"192.168.1.1", true},
		{"169.254.169.254", true}, {"0.0.0.0", true}, {"::1", true}, {"fc00::1", true}, {"fe80::1", true},
		{"1.1.1.1", false}, {"8.8.8.8", false}, {"2606:4700:4700::1111", false},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			if got := isUnsafeAttachmentIP(net.ParseIP(test.raw)); got != test.unsafe {
				t.Fatalf("unsafe = %v, want %v", got, test.unsafe)
			}
		})
	}
}
