package detector

import "testing"

func BenchmarkDetectCommitment(b *testing.B) {
	d := NewDetector()
	message := "Once the build finishes, I'll check back in 5 minutes"

	b.ResetTimer()
	for b.Loop() {
		_ = d.DetectCommitment(message)
	}
}

func BenchmarkNewDetectorAndDetectCommitment(b *testing.B) {
	message := "I'll check back in 5 minutes"

	b.ResetTimer()
	for b.Loop() {
		d := NewDetector()
		_ = d.DetectCommitment(message)
	}
}
