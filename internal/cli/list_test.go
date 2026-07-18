package cli

import "testing"

func TestRunListPlain(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/list"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	listPorcelain = false
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestRunListPorcelain(t *testing.T) {
	withYes(t)
	newTestRepo(t)
	if err := runAdd(addCmd, []string{"feature/porc"}); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	listPorcelain = true
	t.Cleanup(func() { listPorcelain = false })
	if err := runList(); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestRunListNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runList(); err == nil {
		t.Fatal("runList outside a repo: want error, got nil")
	}
}

func TestShortHead(t *testing.T) {
	cases := map[string]string{
		"":    "",
		"abc": "abc",
		"1234567890abcdef1234567890abcdef12345678": "1234567",
	}
	for in, want := range cases {
		if got := shortHead(in); got != want {
			t.Errorf("shortHead(%q) = %q, want %q", in, got, want)
		}
	}
}
