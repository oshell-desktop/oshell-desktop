package core

import "testing"

func BenchmarkSessionSearchNormalization(b *testing.B) {
	for _, text := range []string{"server-123 db.example.com deploy /team SSH 22", "Café ＰＲＯＤ admin /生产 SSH 22"} {
		b.Run(text, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				NormalizeSessionSearch(text)
			}
		})
	}
}
