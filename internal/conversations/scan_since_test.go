package conversations

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestScanner_ScanningSinceNamesTheRunningRead(t *testing.T) {
	reads := map[string]func(*Scanner){
		"scan": func(s *Scanner) { s.Scan(context.Background(), ScanOptions{}) },
		"find": func(s *Scanner) { s.Find(context.Background(), ToolClaude, "missing", ScanOptions{}) },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
			if since := scanner.ScanningSince(); !since.IsZero() {
				t.Fatalf("before any read, ScanningSince = %v; want zero", since)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var hold sync.Once
			clock := scanner.Now
			scanner.Now = func() time.Time {
				hold.Do(func() {
					close(entered)
					<-release
				})
				return clock()
			}
			begun := time.Now()
			done := make(chan struct{})
			go func() {
				read(scanner)
				close(done)
			}()
			<-entered

			since := scanner.ScanningSince()
			close(release)
			<-done

			if since.Before(begun.Add(-time.Second)) || since.After(time.Now()) {
				t.Errorf("during the read, ScanningSince = %v; want when it began, near %v", since, begun)
			}
			if after := scanner.ScanningSince(); !after.IsZero() {
				t.Errorf("after the read, ScanningSince = %v; want zero", after)
			}
		})
	}
}
