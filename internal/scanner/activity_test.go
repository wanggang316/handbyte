package scanner

import "testing"

func TestProjectOwnerFamiliesAreScoped(t *testing.T) {
	rust := Item{Source: "artifact:target", Owner: "Rust"}
	if state, _ := activityFromProcesses(rust, "/usr/local/bin/node\n"); state != Idle {
		t.Fatal("unrelated Node process blocks Rust output")
	}
	if state, _ := activityFromProcesses(rust, "/usr/local/bin/cargo\n"); state != Active {
		t.Fatal("Cargo build not detected")
	}
	python := Item{Source: "artifact:.venv", Owner: "Python"}
	if state, _ := activityFromProcesses(python, "/usr/local/bin/python3.13\n"); state != Active {
		t.Fatal("versioned Python not detected")
	}
}
