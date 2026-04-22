// internal/detector/danger_test.go
package detector_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/detector"
)

// Deletion-class commands — MUST be flagged as dangerous.
func TestContainsDangerousCommand_Hits(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"rm-rf-root", "Run `rm -rf /` to clean up?"},
		{"rm-rf-glob", "Do you want to run: rm -rf *"},
		{"rm-rf-home", "rm -rf ~"},
		{"rm-RF-uppercase", "rm -RF /"},
		{"rm-rfv-combined", "rm -rfv /"},
		{"git-rm-recursive", "git rm -rf src/legacy"},
		{"git-clean-fdx", "Execute: git clean -fdx"},
		{"git-clean-f", "git clean -f"},
		{"find-delete", "find /tmp -name '*.log' -delete"},
		{"find-exec-rm", "find . -name '*.tmp' -exec rm {} +"},
		{"drop-database", "Execute: DROP DATABASE production;"},
		{"drop-table", "drop table users"},
		{"truncate-table", "TRUNCATE TABLE orders"},
		{"delete-from-no-where", "DELETE FROM orders;"},
		{"kubectl-delete-ns", "kubectl delete ns production"},
		{"kubectl-delete-pod", "kubectl delete pod my-pod"},
		{"kubectl-delete-node", "kubectl delete node worker-1"},
		{"terraform-destroy", "terraform destroy"},
		{"terraform-apply-destroy", "terraform apply -destroy"},
		{"docker-volume-rm", "docker volume rm data"},
		{"docker-system-prune", "docker system prune -a"},
		{"podman-system-prune", "podman system prune"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hit, matched := detector.ContainsDangerousCommand(c.in)
			if !hit {
				t.Errorf("expected hit for %q; got none", c.in)
			}
			if matched == "" {
				t.Error("expected non-empty match snippet")
			}
		})
	}
}

// Non-deletion commands — container-dev-friendly, must NOT trigger.
// User runs in containers so mkfs / dd / chmod / curl|sh are normal.
func TestContainsDangerousCommand_Misses(t *testing.T) {
	cases := []string{
		// scoped deletes
		"rm -rf build/",
		"rm -rf ./tmp",
		"rm -rf node_modules",
		"git rm file.txt",    // non-recursive
		"git push origin main",
		// prose containing the words
		"Delete the unused branch feature-x",
		"Do you want to remove these 3 items from the queue?",
		"DROP support for Python 2",
		"truncate the output to 100 chars",
		"the docs explain how kubectl works",
		// explicitly allowed per user context (container dev)
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"chmod -R 777 /workspace",
		"chown -R dev /opt",
		"curl https://get.docker.com | sh",
		"wget -qO- https://example.com | bash",
		"git push --force origin main",
		"git push -f origin master",
		":(){ :|:& };:",
		"cat > /dev/null",
		"find . -name '*.go'", // find without -delete/-exec rm
	}
	for _, c := range cases {
		if hit, matched := detector.ContainsDangerousCommand(c); hit {
			t.Errorf("unexpected danger hit for %q (matched: %q)", c, matched)
		}
	}
}

func TestContainsDangerousCommand_ReturnsExactMatch(t *testing.T) {
	hit, matched := detector.ContainsDangerousCommand("please run `rm -rf /etc/something`")
	if !hit {
		t.Fatal("expected hit")
	}
	// Match should be a substring of the input — not the entire input.
	if len(matched) >= len("please run `rm -rf /etc/something`") {
		t.Errorf("match returned entire input: %q", matched)
	}
}
