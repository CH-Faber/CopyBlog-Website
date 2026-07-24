package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gosync/config"
)

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Command '%s %v' failed: %v\nOutput: %s\n", name, args, err, string(output))
		return err
	}
	return nil
}

func fileHash(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true, nil
}

func approvedPath(cfg *config.Config, name string) (string, string, error) {
	postsRoot := filepath.Clean(cfg.LocalPostsDir)
	target := filepath.Clean(filepath.Join(postsRoot, name))
	rel, err := filepath.Rel(cfg.ProjectRootDir, target)
	if err != nil {
		return "", "", err
	}
	allowedPost, _ := filepath.Rel(postsRoot, target)
	isPost := allowedPost != "." && !strings.HasPrefix(allowedPost, ".."+string(filepath.Separator)) && !filepath.IsAbs(allowedPost)
	taxonomyPath := filepath.Clean(filepath.Join(cfg.ProjectRootDir, "src", "data", "content-taxonomy.json"))
	if !isPost && target != taxonomyPath {
		return "", "", fmt.Errorf("publish path is outside the approved content roots: %s", name)
	}
	return target, filepath.ToSlash(rel), nil
}

// PublishFiles applies only the explicitly approved files and pushes deploy. The
// expected hashes prevent a review job from overwriting a newer published article.
func PublishFiles(cfg *config.Config, files map[string]*[]byte, expected map[string]string, commitMessage string) (string, error) {
	if gitWorktreeDirty(cfg.ProjectRootDir) {
		return "", fmt.Errorf("git worktree is dirty; refusing to mix unreviewed changes into a publish")
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "fetch", "origin", "deploy"); err != nil {
		return "", err
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "checkout", "deploy"); err != nil {
		return "", err
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "rebase", "origin/deploy"); err != nil {
		return "", err
	}

	type resolvedFile struct {
		path string
		rel  string
		data *[]byte
		key  string
	}
	resolved := []resolvedFile{}
	for key, data := range files {
		path, rel, err := approvedPath(cfg, key)
		if err != nil {
			return "", err
		}
		if expectedHash, shouldCheck := expected[key]; shouldCheck {
			currentHash, exists, hashErr := fileHash(path)
			if hashErr != nil {
				return "", hashErr
			}
			desiredAlreadyApplied := false
			if data == nil {
				desiredAlreadyApplied = !exists
			} else if exists {
				desiredSum := sha256.Sum256(*data)
				desiredAlreadyApplied = currentHash == hex.EncodeToString(desiredSum[:])
			}
			if expectedHash == "" && exists && !desiredAlreadyApplied {
				return "", fmt.Errorf("publish conflict: %s was created after review started", key)
			}
			if expectedHash != "" && (!exists || currentHash != expectedHash) && !desiredAlreadyApplied {
				return "", fmt.Errorf("publish conflict: %s changed after review started", key)
			}
		}
		resolved = append(resolved, resolvedFile{path: path, rel: rel, data: data, key: key})
	}

	for _, file := range resolved {
		if file.data == nil {
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return "", err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(file.path), 0755); err != nil {
				return "", err
			}
			if err := os.WriteFile(file.path, *file.data, 0644); err != nil {
				return "", err
			}
		}
		if err := runCommand(cfg.ProjectRootDir, "git", "add", "--", file.rel); err != nil {
			return "", err
		}
	}

	if !gitHasStagedChanges(cfg.ProjectRootDir) {
		if err := runCommand(cfg.ProjectRootDir, "git", "push", "origin", "deploy"); err != nil {
			return "", err
		}
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = cfg.ProjectRootDir
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "commit", "-m", commitMessage); err != nil {
		return "", err
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "push", "origin", "deploy"); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = cfg.ProjectRootDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitWorktreeDirty(dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// gitHasStagedChanges：索引区是否有暂存变更（不打印失败日志；无暂存时 diff --cached 退出码 0）。
func gitHasStagedChanges(dir string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	return cmd.Run() != nil
}

// rebaseOntoOriginDeploy 等价于 pull --rebase，但不依赖 git pull.rebase 全局配置（Git 2.27+ 否则可能报错）。
func rebaseOntoOriginDeploy(dir string) error {
	if err := runCommand(dir, "git", "fetch", "origin", "deploy"); err != nil {
		return err
	}
	if err := runCommand(dir, "git", "rebase", "origin/deploy"); err != nil {
		return err
	}
	return nil
}

func PushToGit(cfg *config.Config) error {
	log.Println("Starting Git Push process to 'deploy' branch...")

	if err := runCommand(cfg.ProjectRootDir, "git", "fetch", "origin"); err != nil {
		return err
	}
	if err := runCommand(cfg.ProjectRootDir, "git", "checkout", "deploy"); err != nil {
		if err2 := runCommand(cfg.ProjectRootDir, "git", "checkout", "-B", "deploy", "origin/deploy"); err2 != nil {
			return err2
		}
	}

	// 先与 origin/deploy 对齐：有脏文件时必须先 stash，否则 rebase 会拒绝（并消除 “behind by N commits”）
	preStashed := false
	if gitWorktreeDirty(cfg.ProjectRootDir) {
		log.Println("Worktree dirty before rebase; stashing (gosync:pre-rebase)")
		if err := runCommand(cfg.ProjectRootDir, "git", "stash", "push", "-u", "-m", "gosync:pre-rebase"); err != nil {
			return err
		}
		preStashed = true
	}
	if err := rebaseOntoOriginDeploy(cfg.ProjectRootDir); err != nil {
		if preStashed {
			_ = runCommand(cfg.ProjectRootDir, "git", "stash", "pop")
		}
		return err
	}
	log.Println("git rebase origin/deploy: ok (pre-commit)")
	if preStashed {
		if err := runCommand(cfg.ProjectRootDir, "git", "stash", "pop"); err != nil {
			return fmt.Errorf("git stash pop after pre-rebase failed（可能有冲突，请在本机处理）: %w", err)
		}
	}

	if err := runCommand(cfg.ProjectRootDir, "git", "add", "src/content/posts/"); err != nil {
		return err
	}

	if gitHasStagedChanges(cfg.ProjectRootDir) {
		if err := runCommand(cfg.ProjectRootDir, "git", "commit", "-m", "Auto sync Obsidian posts & generate AI frontmatter"); err != nil {
			return err
		}
	} else {
		log.Println("Git: src/content/posts 无暂存变更，跳过 commit（例如 S3/AI 未改动文章）")
	}

	// 仍有未暂存修改（如 scripts/gosync/build-linux.sh）时，推送前再 stash + rebase，避免与后续 fetch 冲突
	stashed := false
	if gitWorktreeDirty(cfg.ProjectRootDir) {
		log.Println("Worktree has unstaged/untracked changes; stashing before final rebase + push")
		if err := runCommand(cfg.ProjectRootDir, "git", "stash", "push", "-u", "-m", "gosync:auto"); err != nil {
			log.Printf("git stash failed: %v\n", err)
			return err
		}
		stashed = true
	}
	if stashed {
		defer func() {
			if err := runCommand(cfg.ProjectRootDir, "git", "stash", "pop"); err != nil {
				log.Printf("git stash pop failed (请在本机 git stash list 查看): %v\n", err)
			}
		}()
	}

	if err := rebaseOntoOriginDeploy(cfg.ProjectRootDir); err != nil {
		return err
	}
	log.Println("git rebase origin/deploy: ok (pre-push)")

	err := runCommand(cfg.ProjectRootDir, "git", "push", "origin", "deploy")
	if err != nil {
		// 并发同步或远端恰有新提交时，再 rebase 一次后重试推
		log.Println("Push rejected, retrying after fetch + rebase...")
		if err2 := runCommand(cfg.ProjectRootDir, "git", "fetch", "origin"); err2 != nil {
			return err2
		}
		if err2 := rebaseOntoOriginDeploy(cfg.ProjectRootDir); err2 != nil {
			return err2
		}
		if err2 := runCommand(cfg.ProjectRootDir, "git", "push", "origin", "deploy"); err2 != nil {
			return err2
		}
	}

	log.Println("Successfully pushed to deploy branch!")
	return nil
}
