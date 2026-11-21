package coursehistory

import "testing"

func TestUndoRetainsFailedCommandsAndBranches(t *testing.T) {
	s := New()
	for _, e := range []Event{
		{ID: "a", Type: "command", Command: &Command{Program: "bitcoin-cli", Args: []string{"createwallet", "course"}}},
		{ID: "ar", Type: "result", CommandID: "a", Result: &Result{ExitCode: 0}},
		{ID: "b", Type: "command", Parent: "a", Command: &Command{Program: "bash", Args: []string{"-c", "touch partial; exit 1"}}},
		{ID: "br", Type: "result", CommandID: "b", Result: &Result{ExitCode: 1}},
		{ID: "undo", Type: "checkout", Head: "a"},
		{ID: "c", Type: "command", Parent: "a", Command: &Command{Program: "pwd"}},
		{ID: "cr", Type: "result", CommandID: "c", Result: &Result{}},
	} {
		if err := s.Apply(e); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
	}
	if s.Head != "c" || len(s.Commands) != 3 || s.Commands["b"].Result.ExitCode != 1 {
		t.Fatal("lost branch or failed command")
	}
	if s.Apply(Event{ID: "bad", Type: "checkout", Head: "b"}) == nil {
		t.Fatal("allowed unrelated branch checkout")
	}
}

func TestInterruptedCommandDoesNotAdvanceHead(t *testing.T) {
	s := New()
	if err := s.Apply(Event{ID: "a", Type: "command", Command: &Command{Program: "sleep", Args: []string{"30"}}}); err != nil {
		t.Fatal(err)
	}
	if s.Apply(Event{ID: "b", Type: "command", Command: &Command{Program: "pwd"}}) == nil {
		t.Fatal("allowed concurrent command")
	}
	if s.Apply(Event{ID: "r", Type: "result", CommandID: "other", Result: &Result{}}) == nil {
		t.Fatal("allowed mismatched result")
	}
	if err := s.Apply(Event{ID: "i", Type: "interrupted", CommandID: "a"}); err != nil {
		t.Fatal(err)
	}
	if s.Head != "" || s.Pending != "" || !s.Commands["a"].Interrupted {
		t.Fatal("incorrect recovery")
	}
}
