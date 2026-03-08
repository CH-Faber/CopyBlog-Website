package builder

import (
	"log"
	"os/exec"
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

func PushToGit(cfg *config.Config) error {
	log.Println("Starting Git Push process to 'deploy' branch...")
	
	// Optional: Ensure we are on deploy branch. Skip if already on deploy or causes issues parsing.
	// runCommand(cfg.ProjectRootDir, "git", "checkout", "deploy")
	
	err := runCommand(cfg.ProjectRootDir, "git", "add", "src/content/posts/")
	if err != nil {
		return err
	}
	
	err = runCommand(cfg.ProjectRootDir, "git", "commit", "-m", "Auto sync Obsidian posts & generate AI frontmatter")
	if err != nil {
		// It's possible there are no changes to commit.
		log.Println("Git commit returned error (possibly no changes). Proceeding anyway.")
	}

	err = runCommand(cfg.ProjectRootDir, "git", "push", "origin", "deploy")
	if err != nil {
		return err
	}
	
	log.Println("Successfully pushed to deploy branch!")
	return nil
}
