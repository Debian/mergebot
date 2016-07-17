package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Debian/mergebot/loggedexec"
)

var (
	sourcePackage = flag.String("source_package", "", "Debian source package against which the bug specified in -bug was filed.")
	bug           = flag.String("bug", "", "Debian bug number containing the patch to merge (e.g. 831331 or #831331)")

	filterChangelogMode = flag.Bool("filter_changelog", false, "Not for interactive usage, will be removed! Run in filter changelog mode to work around a gbp dch issue.")
)

// newCommand will be overwritten by mergeAndBuild() once the
// temporary directory is created.
var newCommand = loggedexec.Command

const (
	patchFileName = "latest.patch"
)

func repositoryFor(sourcePackage string) (string, string, error) {
	cmd := newCommand("debcheckout", "--print", sourcePackage)
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "\t")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("Unexpected command output: %v returned %q (split into %v), expected 2 parts", cmd.Args, string(output), parts)
	}
	scm := parts[0]
	url := parts[1]
	if strings.Contains(url, "anonscm.debian.org") {
		url = strings.Replace(url, "git", "git+ssh", 1)
		url = strings.Replace(url, "anonscm.debian.org", "git.debian.org", 1)
		url = strings.Replace(url, "debian.org", "debian.org/git", 1)
	}
	return scm, url, nil
}

func gitCheckout(dst, src string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd := newCommand("gbp", "clone", "--pristine-tar", src, dst)
	cmd.Dir = wd
	if err := cmd.Run(); err != nil {
		return err
	}

	gitConfigArgs := [][]string{
		// Push all (matching) branches at once.
		[]string{"push.default", "matching"},
		// Push tags automatically.
		[]string{"--add", "remote.origin.push", "+refs/heads/*:refs/heads/*"},
		[]string{"--add", "remote.origin.push", "+refs/tags/*:refs/tags/*"},
	}

	if debfullname := os.Getenv("DEBFULLNAME"); debfullname != "" {
		gitConfigArgs = append(gitConfigArgs, []string{"user.name", debfullname})
	}

	if debemail := os.Getenv("DEBEMAIL"); debemail != "" {
		gitConfigArgs = append(gitConfigArgs, []string{"user.email", debemail})
	}

	for _, configArgs := range gitConfigArgs {
		gitArgs := append([]string{"config"}, configArgs...)
		if err := newCommand("git", gitArgs...).Run(); err != nil {
			return err
		}
	}

	return nil
}

// TODO: use git am for git format patches to respect the user’s commit metadata
func applyPatch() error {
	return newCommand("patch", "-p1", "-i", filepath.Join("..", patchFileName)).Run()
}

func gitCommit(author, message string) error {
	if err := newCommand("git", "add", ".").Run(); err != nil {
		return err
	}

	return newCommand("git", "commit", "-a",
		"--author", author,
		"--message", message).Run()
}

func sha256of(path string) (string, error) {
	h := sha256.New()

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%.16x", h.Sum(nil)), nil
}

// TODO: if gbp dch returns with “Version %s not found”, that’s fine, as the changelog is already up to date. Can we detect this case, or change our gbp dch invocation to not complain?
func releaseChangelog() error {
	cmd := newCommand("gbp", "dch", "--release", "--git-author", "--commit")
	// See the comment on filterChangelog() for details:
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		return err
	}
	cmd.Env = append(cmd.Env, []string{
		// Set VISUAL because gbp dch has no flag to specify the editor.
		// Ideally we’d set this to /bin/true, but we need to filter the changelog because “gbp dch” generates an empty entry.
		fmt.Sprintf("VISUAL=%s -filter_changelog", self),
	}...)
	return cmd.Run()
}

func buildPackage() error {
	return newCommand("gbp", "buildpackage",
		// Tag debian/%(version)s after building successfully.
		"--git-tag",
		// Build in a separate directory to avoid modifying the git checkout.
		"--git-export-dir=../export",
		"--git-builder=sbuild -v -As --dist=unstable").Run()
}

// mergeAndBuild downloads the most recent patch in the specified bug
// from the BTS, checks out the package’s packaging repository, merges
// the patch and builds the package.
func mergeAndBuild(url string) (string, error) {
	tempDir, err := ioutil.TempDir("", "mergebot-")
	if err != nil {
		return tempDir, err
	}
	newCommand = func(name string, arg ...string) *loggedexec.LoggedCmd {
		cmd := loggedexec.Command(name, arg...)
		cmd.LogDir = tempDir
		cmd.Logger = log.New(os.Stderr, "", log.LstdFlags)
		// TODO: copy passthroughEnv() from dh-make-golang/make.go
		for _, variable := range []string{"DEBFULLNAME", "DEBEMAIL", "SSH_AGENT_PID", "GPG_AGENT_INFO", "SSH_AUTH_SOCK"} {
			if value, ok := os.LookupEnv(variable); ok {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", variable, value))
			}
		}
		return cmd
	}

	patch, err := getMostRecentPatch(url, *bug)
	if err != nil {
		return tempDir, err
	}

	if err := ioutil.WriteFile(filepath.Join(tempDir, patchFileName), patch.Data, 0600); err != nil {
		return tempDir, err
	}

	scm, url, err := repositoryFor(*sourcePackage)
	if err != nil {
		return tempDir, err
	}
	if scm != "git" {
		return tempDir, fmt.Errorf("mergebot only supports git currently, but %q is using the SCM %q", url, scm)
	}

	checkoutDir := filepath.Join(tempDir, "repo")

	// Make every command run in checkoutDir by default from now on.
	previousNewCommand := newCommand
	newCommand = func(name string, arg ...string) *loggedexec.LoggedCmd {
		cmd := previousNewCommand(name, arg...)
		cmd.Dir = checkoutDir
		return cmd
	}

	if err := gitCheckout(checkoutDir, url); err != nil {
		return tempDir, err
	}

	// TODO: edge case: the user might supply a patch which touches changelog but doesn’t include Closes: #bugnumber. In that case, we should modify the changelog accordingly (e.g. using debchange --closes?)

	changelogPath := filepath.Join(checkoutDir, "debian", "changelog")
	oldChangelogSum, err := sha256of(changelogPath)
	if err != nil {
		return tempDir, err
	}

	if err := applyPatch(); err != nil {
		return tempDir, err
	}

	patchCommitMessage := fmt.Sprintf("Fix for “%s” (Closes: #%s)", patch.Subject, *bug)
	if err := gitCommit(patch.Author, patchCommitMessage); err != nil {
		return tempDir, err
	}

	newChangelogSum, err := sha256of(changelogPath)
	if err != nil {
		return tempDir, err
	}
	if newChangelogSum != oldChangelogSum {
		log.Printf("%q changed", changelogPath) // TODO: remove in case we can make releaseChangelog() always work
	}

	if err := releaseChangelog(); err != nil {
		return tempDir, err
	}

	if err := buildPackage(); err != nil {
		return tempDir, err
	}

	// TODO: run lintian, include result in report

	return tempDir, nil
}

func main() {
	flag.Parse()

	if *filterChangelogMode {
		if flag.NArg() != 1 {
			log.Fatalf("Syntax: %s -filter_changelog <path>", os.Args[0])
		}

		if err := filterChangelog(flag.Arg(0)); err != nil {
			log.Fatal(err)
		}
		return
	}

	*bug = strings.TrimPrefix(*bug, "#")

	// TODO: infer sourcePackage from --bug
	log.Printf("will work on package %q, bug %q", *sourcePackage, *bug)

	tempDir, err := mergeAndBuild(soapAddress)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Merge and build successful!")
	log.Printf("Please introspect the resulting Debian package and git repository, then push and upload:")
	log.Printf("cd %q", tempDir)
	log.Printf("(cd repo && git push)")
	log.Printf("(cd export && debsign *.changes && dput *.changes)")
}
