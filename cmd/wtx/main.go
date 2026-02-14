package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type config struct {
	MainBranch        string   `json:"mainBranch"`
	DefaultBaseBranch string   `json:"defaultBaseBranch"`
	WorktreesDir      string   `json:"worktreesDir"`
	EnvFiles          []string `json:"envFiles"`
	FrontendDir       string   `json:"frontendDir"`
	BackendDir        string   `json:"backendDir"`
	FrontendInstall   []string `json:"frontendInstallCmd"`
	BackendInstall    []string `json:"backendInstallCmd"`
	LLM               llmCfg   `json:"llm"`
}

type llmCfg struct {
	Default                  string                    `json:"default"`
	Allowed                  []string                  `json:"allowed"`
	BranchNamePromptTemplate string                    `json:"branchNamePromptTemplate"`
	Commands                 map[string]llmCommandCfg  `json:"commands"`
}

type llmCommandCfg struct {
	BranchNameArgsTemplate []string `json:"branchNameArgsTemplate"`
	TaskRunArgsTemplate    []string `json:"taskRunArgsTemplate"`
}

type worktreeEntry struct {
	path   string
	branch string
}

func main() {
	configPath := resolveConfigPath()
	cfg, err := loadConfig(configPath)
	if err != nil {
		fatal(err)
	}

	if len(os.Args) < 2 {
		fatal(errors.New("usage: wtx <start|new-worktree|clean> [args...]"))
	}

	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "start":
		err = runStart(cfg, args)
	case "new-worktree":
		err = runNewWorktree(cfg, args, true)
	case "clean":
		err = runClean(cfg)
	default:
		err = fmt.Errorf("unknown command: %s", sub)
	}
	if err != nil {
		fatal(err)
	}
}

func runStart(cfg config, args []string) error {
	var task string
	base := cfg.DefaultBaseBranch
	llm := ""

	switch len(args) {
	case 0:
		task = promptRequired("作業内容: ")
		base = promptDefault("ベースブランチ ["+cfg.DefaultBaseBranch+"]: ", cfg.DefaultBaseBranch)
		llm = promptOptional("AIを選択してください (codex/claude): ")
	case 1:
		v := strings.ToLower(strings.TrimSpace(args[0]))
		if isAllowedLLM(cfg, v) {
			llm = v
			task = promptRequired("作業内容: ")
			base = promptDefault("ベースブランチ ["+cfg.DefaultBaseBranch+"]: ", cfg.DefaultBaseBranch)
		} else {
			task = args[0]
		}
	case 2:
		task = args[0]
		base = args[1]
	default:
		task = args[0]
		base = args[1]
		llm = args[2]
	}

	if strings.TrimSpace(task) == "" {
		return errors.New("no description provided")
	}
	if strings.TrimSpace(base) == "" {
		base = cfg.DefaultBaseBranch
	}

	llm = normalizeLLM(cfg, llm)
	if llm == "" {
		llm = normalizeLLM(cfg, promptOptional("AIを選択してください (codex/claude): "))
	}
	if llm == "" {
		return fmt.Errorf("invalid AI selection (expected one of: %s)", strings.Join(cfg.LLM.Allowed, ", "))
	}

	return createWorktree(cfg, task, base, llm, true)
}

func runNewWorktree(cfg config, args []string, runTask bool) error {
	var task string
	base := cfg.DefaultBaseBranch
	llm := cfg.LLM.Default

	if len(args) >= 1 {
		task = args[0]
	}
	if len(args) >= 2 {
		base = args[1]
	}
	if len(args) >= 3 {
		llm = args[2]
	}

	if strings.TrimSpace(task) == "" {
		task = promptRequired("作業内容: ")
		base = promptDefault("ベースブランチ ["+cfg.DefaultBaseBranch+"]: ", cfg.DefaultBaseBranch)
		llm = promptDefault("AIを選択してください (codex/claude) ["+cfg.LLM.Default+"]: ", cfg.LLM.Default)
	}

	if strings.TrimSpace(base) == "" {
		base = cfg.DefaultBaseBranch
	}
	llm = normalizeLLM(cfg, llm)
	if llm == "" {
		return fmt.Errorf("invalid AI selection (expected one of: %s)", strings.Join(cfg.LLM.Allowed, ", "))
	}
	return createWorktree(cfg, task, base, llm, runTask)
}

func createWorktree(cfg config, task, base, llm string, runTask bool) error {
	if err := requireCmd("git"); err != nil {
		return err
	}
	if _, err := runCmdCapture("", "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return errors.New("not inside a git repository")
	}

	repoRootRaw, err := runCmdCapture("", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	repoRoot := strings.TrimSpace(repoRootRaw)

	rawBranch := generateBranchName(cfg, task, llm)
	branch := sanitizeBranch(rawBranch)
	if branch == "" {
		return errors.New("empty branch name after sanitize")
	}

	worktreesDir := filepath.Join(repoRoot, cfg.WorktreesDir)
	targetPath := filepath.Join(worktreesDir, strings.ReplaceAll(branch, "/", "__"))

	if err := runCmdStream("", "git", "fetch", "origin", base, "--prune"); err != nil {
		return err
	}
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		return err
	}

	remoteExists := runCmd("git", "ls-remote", "--exit-code", "--heads", "origin", branch) == nil
	if remoteExists {
		if err := runCmdStream("", "git", "worktree", "add", "--checkout", targetPath, "origin/"+branch); err != nil {
			return err
		}
		if err := runCmd("git", "-C", targetPath, "switch", "-c", branch); err != nil {
			if err2 := runCmdStream("", "git", "-C", targetPath, "switch", branch); err2 != nil {
				return err2
			}
		}
	} else {
		if err := runCmdStream("", "git", "worktree", "add", "-b", branch, targetPath, "origin/"+base); err != nil {
			return err
		}
	}

	_ = runCmd("git", "-C", targetPath, "branch", "--unset-upstream")
	if err := runCmdStream("", "git", "-C", targetPath, "push", "-u", "origin", branch+":"+branch); err != nil {
		return err
	}

	fmt.Println("Copying environment files...")
	for _, envFile := range cfg.EnvFiles {
		src := filepath.Join(repoRoot, envFile)
		dst := filepath.Join(targetPath, envFile)
		if _, err := os.Stat(src); err != nil {
			fmt.Printf("Not found: %s (skipped)\n", envFile)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
		fmt.Printf("Copied: %s\n", envFile)
	}

	fmt.Println("Installing dependencies...")
	frontendPath := filepath.Join(targetPath, cfg.FrontendDir)
	if isDir(frontendPath) && len(cfg.FrontendInstall) > 0 {
		fmt.Println("Installing frontend dependencies...")
		if err := runCmdStream(frontendPath, cfg.FrontendInstall[0], cfg.FrontendInstall[1:]...); err != nil {
			return err
		}
	} else {
		fmt.Println("Frontend install skipped")
	}

	backendPath := filepath.Join(targetPath, cfg.BackendDir)
	if isDir(backendPath) && len(cfg.BackendInstall) > 0 {
		fmt.Println("Installing backend dependencies...")
		if err := runCmdStream(backendPath, cfg.BackendInstall[0], cfg.BackendInstall[1:]...); err != nil {
			return err
		}
	} else {
		fmt.Println("Backend install skipped")
	}

	fmt.Printf("Worktree created at: %s\n", targetPath)
	fmt.Printf("Branch: %s (base: origin/%s)\n", branch, base)
	fmt.Printf("Upstream: origin/%s\n", branch)

	if runTask {
		fmt.Printf("Running %s with task prompt...\n", llm)
		if err := runLLMTask(cfg, llm, targetPath, task); err != nil {
			return err
		}
	}
	return nil
}

func runClean(cfg config) error {
	if err := requireCmd("git"); err != nil {
		return err
	}
	listRaw, err := runCmdCapture("", "git", "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	entries := parseWorktreeList(listRaw)
	if len(entries) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	mainWorktree := entries[0].path
	fmt.Println("Checking for merged worktrees...")
	for _, e := range entries {
		branch := strings.TrimPrefix(e.branch, "refs/heads/")
		if branch == "" || branch == cfg.MainBranch {
			continue
		}

		merged := runCmdIn(mainWorktree, "git", "merge-base", "--is-ancestor", branch, cfg.MainBranch) == nil
		if !merged {
			fmt.Printf("Branch '%s' is not merged yet. Keeping worktree.\n", branch)
			continue
		}

		fmt.Printf("Branch '%s' is merged. Removing worktree at '%s'...\n", branch, e.path)
		if err := runCmdStream("", "git", "worktree", "remove", e.path, "--force"); err != nil {
			return err
		}
		if err := runCmd("git", "branch", "-d", branch); err != nil {
			if err2 := runCmdStream("", "git", "branch", "-D", branch); err2 != nil {
				return err2
			}
		}
		fmt.Printf("Removed worktree and branch: %s\n", branch)
	}
	fmt.Println("Done cleaning merged worktrees.")
	return nil
}

func generateBranchName(cfg config, task, llm string) string {
	prompt := strings.ReplaceAll(cfg.LLM.BranchNamePromptTemplate, "{task}", task)
	aiCfg, ok := cfg.LLM.Commands[llm]
	if ok && commandExists(llm) && len(aiCfg.BranchNameArgsTemplate) > 0 {
		args := replaceTemplates(aiCfg.BranchNameArgsTemplate, map[string]string{
			"{prompt}": prompt,
			"{task}":   task,
		})
		out, err := runCmdCapture("", llm, args...)
		if err == nil {
			v := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(out, "\r", ""), "\n", ""))
			if v != "" {
				return v
			}
		}
	}

	fallback := strings.ToLower(task)
	fallback = regexpReplace(fallback, `[^a-z0-9]+`, "-")
	fallback = strings.Trim(fallback, "-")
	if fallback == "" {
		fallback = "task"
	}
	return fallback
}

func runLLMTask(cfg config, llm, worktreePath, task string) error {
	aiCfg, ok := cfg.LLM.Commands[llm]
	if !ok {
		return fmt.Errorf("missing LLM command config for: %s", llm)
	}
	if !commandExists(llm) {
		fmt.Printf("%s not found. Skip auto-run.\n", llm)
		return nil
	}
	args := replaceTemplates(aiCfg.TaskRunArgsTemplate, map[string]string{
		"{task}": task,
	})
	if len(args) == 0 {
		return fmt.Errorf("empty taskRunArgsTemplate for %s", llm)
	}
	return runCmdStream(worktreePath, llm, args...)
}

func sanitizeBranch(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	s = regexpReplace(s, `[^a-z0-9/-]+`, "-")
	s = regexpReplace(s, `-+`, "-")
	s = regexpReplace(s, `/+`, "/")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}

	prefixOK := strings.HasPrefix(s, "feature/") ||
		strings.HasPrefix(s, "bugfix/") ||
		strings.HasPrefix(s, "fix/") ||
		strings.HasPrefix(s, "chore/") ||
		strings.HasPrefix(s, "refactor/")
	if !prefixOK {
		s = "feature/" + s
	}
	return s
}

func loadConfig(path string) (config, error) {
	var cfg config
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	if cfg.DefaultBaseBranch == "" {
		cfg.DefaultBaseBranch = "develop"
	}
	if cfg.MainBranch == "" {
		cfg.MainBranch = cfg.DefaultBaseBranch
	}
	if cfg.LLM.Default == "" {
		cfg.LLM.Default = "codex"
	}
	return cfg, nil
}

func resolveConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("TMYJOE_CONFIG")); v != "" {
		return v
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	if _, err := os.Stat(".tmyjoe/wtx/config.json"); err == nil {
		return ".tmyjoe/wtx/config.json"
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ".tmyjoe/wtx/config.json"
}

func parseWorktreeList(raw string) []worktreeEntry {
	var out []worktreeEntry
	var cur worktreeEntry
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if cur.path != "" {
				out = append(out, cur)
			}
			cur = worktreeEntry{}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			cur.path = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch ") {
			cur.branch = strings.TrimPrefix(line, "branch ")
		}
	}
	if cur.path != "" {
		out = append(out, cur)
	}
	return out
}

func runCmdCapture(dir, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if dir != "" {
		cmd.Dir = dir
	}
	err := cmd.Run()
	return buf.String(), err
}

func runCmdStream(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func runCmdIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Dir = dir
	return cmd.Run()
}

func requireCmd(name string) error {
	if !commandExists(name) {
		return fmt.Errorf("required command not found: %s", name)
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func replaceTemplates(values []string, vars map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		r := v
		for k, x := range vars {
			r = strings.ReplaceAll(r, k, x)
		}
		out = append(out, r)
	}
	return out
}

func promptOptional(label string) string {
	fmt.Print(label)
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(s)
}

func promptDefault(label, defaultValue string) string {
	v := promptOptional(label)
	if strings.TrimSpace(v) == "" {
		return defaultValue
	}
	return v
}

func promptRequired(label string) string {
	v := promptOptional(label)
	if strings.TrimSpace(v) == "" {
		fatal(errors.New("no description provided"))
	}
	return v
}

func isAllowedLLM(cfg config, v string) bool {
	for _, x := range cfg.LLM.Allowed {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func normalizeLLM(cfg config, llm string) string {
	v := strings.ToLower(strings.TrimSpace(llm))
	if v == "" {
		return ""
	}
	if isAllowedLLM(cfg, v) {
		return v
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func regexpReplace(in, pattern, replacement string) string {
	re := regexpMustCompile(pattern)
	return re.ReplaceAllString(in, replacement)
}

func regexpMustCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}
