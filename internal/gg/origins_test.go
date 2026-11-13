package gg

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSourceMigrationOnlyRetiresConfirmedInactiveRecords(t *testing.T) {
	for _, test := range []struct {
		name, output  string
		exit, sources int
	}{
		{"absent", "error connecting to /tmp/fixture (No such file or directory)", 1, 0},
		{"stopped", "no server running on /tmp/fixture", 1, 0},
		{"permission", "error connecting to /tmp/fixture (Permission denied)", 1, 2},
		{"malformed", "unexpected output", 0, 2},
		{"running", "%1\t@1\t0\t\t0\t1\tsh\tdashboard\t0\t0\t80\t24\t80\t24\t\t12345\t", 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			p := &Project{Root: root, Dir: filepath.Join(root, "registry"), Socket: "gg-migration-fixture"}
			if err := os.Mkdir(p.Dir, 0700); err != nil {
				t.Fatal(err)
			}
			state := &State{NextID: 10, Tasks: []*Task{
				{ID: 0, Protected: true, Branch: "main", Path: root},
				{ID: 7, Protected: true, Branch: "external", Path: filepath.Join(root, "external")},
				{ID: 9, Branch: "feature", Path: root + "__sessions/feature"},
			}}
			file := filepath.Join(p.Dir, "state.json")
			if err := atomicJSON(file, state); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(file)
			programs := t.TempDir()
			output := filepath.Join(programs, "output")
			if err := os.WriteFile(output, []byte(test.output+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\ncase \"$3\" in list-panes) ;; *) exit 99 ;; esac\n/bin/cat " + quote(output) + "\n"
			if test.exit != 0 {
				script += "exit 1\n"
			}
			if err := os.WriteFile(filepath.Join(programs, "tmux"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", programs)
			got, err := p.currentState(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Tasks) != test.sources+1 || got.NextID != 10 || got.Tasks[len(got.Tasks)-1].ID != 9 {
				t.Fatalf("migration lost records/counter: %+v", got)
			}
			after, _ := os.ReadFile(file)
			if test.sources == 2 && string(before) != string(after) {
				t.Fatal("uncertain snapshot changed registry")
			}
			old := time.Unix(1000, 0)
			if err := os.Chtimes(file, old, old); err != nil {
				t.Fatal(err)
			}
			if _, err := p.currentState(root); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(file)
			if !info.ModTime().Equal(old) {
				t.Fatal("unchanged registry rewritten on refresh")
			}
		})
	}
}

func TestEmptyRegistryDoesNotInventSourceOrRequireTmux(t *testing.T) {
	p := &Project{Root: t.TempDir(), Dir: filepath.Join(t.TempDir(), "missing")}
	t.Setenv("PATH", t.TempDir())
	state, err := p.currentState(p.Root)
	if err != nil || len(state.Tasks) != 0 || state.NextID != 1 {
		t.Fatalf("%+v %v", state, err)
	}
	if _, err := os.Stat(p.Dir); !os.IsNotExist(err) {
		t.Fatal("read created registry")
	}
}

func TestSourceStopPreservesRecordWhenTmuxCannotBeRead(t *testing.T) {
	root := t.TempDir()
	p := &Project{Root: root, Dir: t.TempDir(), Socket: "gg-migration-fixture"}
	state := &State{NextID: 1, Tasks: []*Task{{ID: 0, Protected: true, Branch: "main", Path: root}}}
	file := filepath.Join(p.Dir, "state.json")
	if err := atomicJSON(file, state); err != nil {
		t.Fatal(err)
	}
	programs := t.TempDir()
	if err := os.WriteFile(filepath.Join(programs, "tmux"), []byte("#!/bin/sh\necho 'error connecting to /tmp/fixture (Permission denied)'\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", programs)
	if err := p.operate("stop", []int{0}, false, false); err == nil {
		t.Fatal("uncertain stop reported success")
	}
	remaining, err := p.load()
	if err != nil || len(remaining.Tasks) != 1 || remaining.Tasks[0].ID != 0 {
		t.Fatalf("source lost: %+v %v", remaining, err)
	}
}
