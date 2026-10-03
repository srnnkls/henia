package preload

var privileged = []string{"sudo", "doas", "su", "pkexec", "run0"}

var shells = []string{"sh", "bash", "zsh", "dash", "ksh", "mksh", "fish"}

var fileWriters = []string{
	"rm", "rmdir", "mv", "cp", "tee", "touch", "mkdir", "ln", "chmod", "chown", "chgrp",
	"truncate", "dd", "install", "shred", "unlink", "mkfifo", "mknod", "patch", "split", "csplit",
}

var httpie = []string{"http", "https", "xh", "xhs"}

var remoteShells = []string{"ssh", "scp", "sftp", "rsync", "mosh", "telnet"}

var processControl = []string{"kill", "pkill", "killall", "shutdown", "reboot", "halt", "poweroff"}

type wrapper struct {
	valued         []string
	positional     bool
	inspectNothing []string
	uninspectable  []string
}

var wrappers = map[string]*wrapper{
	"env":        {valued: []string{"-u", "--unset", "-C", "--chdir", "-P"}, uninspectable: []string{"-S", "--split-string"}},
	"nice":       {valued: []string{"-n", "--adjustment"}},
	"nohup":      {},
	"time":       {},
	"builtin":    {},
	"command":    {inspectNothing: []string{"-v", "-V"}},
	"exec":       {valued: []string{"-a"}},
	"stdbuf":     {valued: []string{"-i", "-o", "-e"}},
	"caffeinate": {valued: []string{"-t", "-w"}},
	"timeout":    {valued: []string{"-s", "--signal", "-k", "--kill-after"}, positional: true},
	"gtimeout":   {valued: []string{"-s", "--signal", "-k", "--kill-after"}, positional: true},
	"xargs":      {valued: []string{"-I", "-L", "-n", "-P", "-s", "-d", "-E", "-a", "--arg-file", "--delimiter", "--max-args", "--max-procs", "--max-chars", "--max-lines", "--replace", "--eof", "--process-slot-var"}},
}

type family struct {
	rule        string
	effect      string
	valued      []string
	subcommands []string
	nested      map[string][]string
	always      bool
}

var packages = func(subcommands ...string) family {
	return family{rule: "package", effect: "changes installed packages or publishes one", subcommands: subcommands}
}

var subcommandRules = map[string]family{
	"git": {
		rule:   "git",
		effect: "changes the repository or a remote",
		valued: []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env", "--super-prefix"},
		subcommands: []string{
			"push", "commit", "reset", "checkout", "switch", "restore", "merge", "rebase", "pull", "fetch",
			"clone", "init", "add", "rm", "mv", "clean", "cherry-pick", "revert", "am", "apply", "gc",
			"prune", "update-ref", "filter-branch", "send-email",
		},
		nested: map[string][]string{"stash": {"list", "show"}},
	},
	"npm":       packages("install", "i", "add", "ci", "uninstall", "remove", "rm", "un", "update", "up", "upgrade", "publish", "unpublish", "link", "unlink", "dedupe", "prune", "exec", "x", "init", "create", "deprecate", "dist-tag", "owner", "access", "version"),
	"pnpm":      packages("install", "i", "add", "remove", "rm", "uninstall", "update", "up", "upgrade", "publish", "link", "unlink", "dedupe", "prune", "dlx", "exec", "create", "import"),
	"yarn":      packages("install", "add", "remove", "upgrade", "up", "publish", "link", "unlink", "dlx", "exec", "create", "init", "dedupe", "npm"),
	"bun":       packages("install", "i", "add", "a", "remove", "rm", "update", "upgrade", "publish", "link", "unlink", "create", "init", "x"),
	"pip":       packages("install", "uninstall", "download", "wheel"),
	"pip3":      packages("install", "uninstall", "download", "wheel"),
	"pipx":      packages("install", "uninstall", "upgrade", "upgrade-all", "reinstall", "reinstall-all", "inject", "run", "ensurepath"),
	"uv":        packages("add", "remove", "sync", "lock", "pip", "tool", "publish", "venv", "init", "python", "self", "run", "export", "build"),
	"poetry":    packages("add", "remove", "install", "update", "lock", "publish", "build", "init", "new", "self", "run"),
	"cargo":     packages("install", "uninstall", "add", "remove", "publish", "yank", "update", "new", "init", "run", "owner", "login", "logout"),
	"go":        packages("install", "get", "run", "generate", "work", "mod"),
	"gem":       packages("install", "uninstall", "update", "push", "yank", "owner", "cleanup"),
	"brew":      packages("install", "uninstall", "remove", "rm", "reinstall", "upgrade", "update", "tap", "untap", "link", "unlink", "cleanup", "autoremove", "pin", "unpin", "services", "bundle"),
	"apt":       packages("install", "remove", "purge", "upgrade", "full-upgrade", "dist-upgrade", "update", "autoremove"),
	"apt-get":   packages("install", "remove", "purge", "upgrade", "dist-upgrade", "update", "autoremove", "build-dep"),
	"dnf":       packages("install", "remove", "erase", "upgrade", "update", "downgrade", "reinstall", "autoremove", "swap"),
	"yum":       packages("install", "remove", "erase", "upgrade", "update", "downgrade", "reinstall", "autoremove", "swap"),
	"zypper":    packages("install", "in", "remove", "rm", "update", "up", "dist-upgrade", "dup", "patch"),
	"pacman":    {rule: "package", effect: "changes installed packages", always: true},
	"apk":       packages("add", "del", "upgrade", "update", "fix"),
	"nix":       packages("profile", "build", "develop", "run", "shell", "copy", "store", "registry", "upgrade-nix"),
	"nix-env":   {rule: "package", effect: "changes installed packages", always: true},
	"mise":      packages("install", "i", "use", "u", "uninstall", "rm", "upgrade", "up", "prune", "self-update", "plugins", "trust", "settings", "set", "unuse", "run", "exec", "x"),
	"npx":       {rule: "package", effect: "downloads and runs a package", always: true},
	"pnpx":      packages(""),
	"bunx":      packages(""),
	"uvx":       {rule: "package", effect: "downloads and runs a package", always: true},
	"docker":    daemon("run", "exec", "build", "buildx", "builder", "push", "pull", "rm", "rmi", "stop", "kill", "start", "restart", "create", "commit", "tag", "cp", "load", "import", "login", "logout", "pause", "unpause", "rename", "update", "attach", "network", "volume", "system", "compose", "container", "image", "plugin", "swarm", "service", "stack", "secret", "config", "node", "context", "trust", "manifest"),
	"podman":    daemon("run", "exec", "build", "push", "pull", "rm", "rmi", "stop", "kill", "start", "restart", "create", "commit", "tag", "cp", "load", "import", "login", "logout", "pause", "unpause", "rename", "attach", "network", "volume", "system", "compose", "container", "image", "pod", "machine", "play", "kube", "secret", "manifest", "generate"),
	"kubectl":   daemon("apply", "create", "delete", "edit", "patch", "replace", "scale", "rollout", "label", "annotate", "set", "expose", "run", "exec", "cp", "drain", "cordon", "uncordon", "taint", "autoscale", "debug", "attach", "port-forward", "proxy", "certificate", "config"),
	"systemctl": service("start", "stop", "restart", "reload", "try-restart", "reload-or-restart", "enable", "disable", "reenable", "kill", "mask", "unmask", "daemon-reload", "daemon-reexec", "isolate", "poweroff", "reboot", "halt", "suspend", "hibernate", "edit", "set-property", "link", "preset", "preset-all", "revert", "set-default", "set-environment", "unset-environment", "import-environment", "reset-failed", "freeze", "thaw", "clean"),
	"launchctl": service("load", "unload", "start", "stop", "kickstart", "bootstrap", "bootout", "enable", "disable", "kill", "remove", "submit", "setenv", "unsetenv", "config", "reboot"),
	"service":   service("start", "stop", "restart", "reload"),
	"defaults":  {rule: "daemon", effect: "changes macOS preferences", subcommands: []string{"write", "delete", "import", "rename"}},
	"tmux":      {rule: "daemon", effect: "changes the tmux server", valued: []string{"-L", "-S", "-f", "-T"}, subcommands: []string{"send-keys", "send", "kill-server", "kill-session", "kill-window", "killw", "kill-pane", "killp", "new-session", "new", "new-window", "neww", "split-window", "splitw", "run-shell", "run", "respawn-pane", "respawn-window", "rename-session", "rename-window", "set-option", "set", "set-window-option", "setw", "set-environment", "setenv", "source-file", "source", "load-buffer", "paste-buffer", "pipe-pane", "move-window", "swap-pane", "detach-client", "switch-client"}},
}

func daemon(subcommands ...string) family {
	return family{rule: "daemon", effect: "changes state through a daemon", subcommands: subcommands}
}

func service(subcommands ...string) family {
	return family{rule: "process-control", effect: "changes a running service", subcommands: subcommands}
}

var ghMutations = map[string][]string{
	"pr":         {"merge", "create", "new", "edit", "comment", "close", "reopen", "ready", "review", "lock", "unlock", "update-branch", "checkout", "co", "revert"},
	"issue":      {"create", "new", "edit", "comment", "close", "reopen", "delete", "transfer", "lock", "unlock", "pin", "unpin", "develop"},
	"repo":       {"create", "new", "delete", "edit", "fork", "rename", "archive", "unarchive", "sync", "clone", "set-default", "deploy-key", "autolink", "gitignore", "license"},
	"release":    {"create", "new", "delete", "edit", "upload", "delete-asset", "download"},
	"gist":       {"create", "new", "delete", "edit", "clone", "rename"},
	"label":      {"create", "delete", "edit", "clone"},
	"secret":     {"set", "delete", "remove"},
	"variable":   {"set", "delete", "remove"},
	"workflow":   {"run", "enable", "disable"},
	"run":        {"rerun", "cancel", "delete", "download"},
	"cache":      {"delete"},
	"ssh-key":    {"add", "delete"},
	"gpg-key":    {"add", "delete"},
	"project":    {"create", "delete", "edit", "close", "copy", "field-create", "field-delete", "item-add", "item-archive", "item-create", "item-delete", "item-edit", "link", "unlink", "mark-template"},
	"auth":       {"login", "logout", "refresh", "setup-git", "switch"},
	"config":     {"set", "clear-cache"},
	"extension":  {"install", "upgrade", "remove", "create", "exec"},
	"ext":        {"install", "upgrade", "remove", "create", "exec"},
	"alias":      {"set", "delete", "import"},
	"codespace":  {"create", "delete", "edit", "stop", "rebuild", "cp", "ssh", "code", "jupyter", "ports"},
	"cs":         {"create", "delete", "edit", "stop", "rebuild", "cp", "ssh", "code", "jupyter", "ports"},
	"ruleset":    {},
	"agent-task": {"create"},
}
