package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNaverCooldownSurvivesNextProcessRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "naver.json")
	now := time.Now()
	if err := checkNaverCooldown(path, now); err != nil {
		t.Fatal(err)
	}
	until := now.Add(2 * time.Hour)
	if err := saveNaverCooldown(path, until); err != nil {
		t.Fatal(err)
	}
	if err := checkNaverCooldown(path, now.Add(time.Minute)); err == nil {
		t.Fatal("next invocation ignored Retry-After")
	}
	if err := checkNaverCooldown(path, until.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path + ".retry-after")
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatalf("cooldown permissions %v", st.Mode().Perm())
	}
	if err := os.WriteFile(path+".retry-after", []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkNaverCooldown(path, now); err == nil {
		t.Fatal("corrupt cooldown was silently ignored")
	}
}
