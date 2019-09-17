package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/caarlos0/env"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/otiai10/copy"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"knative.dev/eventing-contrib/pkg/kncloudevents"
	"sigs.k8s.io/kustomize/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/pkg/commands/build"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/resource"
)

const (
	basePath = "/home"
)

type config struct {
	KustomizeRepo string `env:"K_REPO"`
	Token         string `env:"GIT_TOKEN"`
}

var cfg config

func main() {
	if err := env.Parse(&cfg); err != nil {
		log.Fatal("env parsing failed: ", err)
	}

	c, err := kncloudevents.NewDefaultClient()
	if err != nil {
		log.Fatal("Failed to create client, ", err)
	}

	log.Fatal(c.StartReceiver(context.Background(), handler))
}

func handler(ctx context.Context, event cloudevents.Event) error {
	// Git clone incoming tag or commit to a target directory
	// Git clone kustomization repository
	// Copy kustomization repository content to the target directory overriding origin files if there are
	// Apply kustomization
	// Push result back to kustomization repository

	repoPath, err := checkoutTargetRepository(event)
	if err != nil {
		return err
	}
	kustomizationPath := fmt.Sprintf("%s-kustomize", repoPath)
	if err := checkoutKustomizationRepository(cfg.KustomizeRepo, kustomizationPath); err != nil {
		return err
	}

	tmpPath := fmt.Sprintf("%s-tmp", repoPath)

	if err := copy.Copy(kustomizationPath, tmpPath); err != nil {
		return err
	}

	if err := copy.Copy(repoPath, tmpPath); err != nil {
		return err
	}

	opt := build.NewOptions(tmpPath, fmt.Sprintf("%s/output.yaml", kustomizationPath))

	uf := kunstruct.NewKunstructuredFactoryImpl()
	pf := transformer.NewFactoryImpl()
	rf := resmap.NewFactory(resource.NewFactory(uf))

	if err := opt.RunBuild(os.Stdout, fs.MakeRealFS(), rf, pf); err != nil {
		return err
	}

	return push(kustomizationPath)
}

func checkoutTargetRepository(event cloudevents.Event) (string, error) {
	meta := event.Extensions()
	owner, exist := meta["Owner"]
	if !exist {
		return "", fmt.Errorf("Owner field is not send in event")
	}
	repository, exist := meta["Repository"]
	if !exist {
		return "", fmt.Errorf("Repository field is not send in event")
	}
	cloneURL, exist := meta["Clone_url"]
	if !exist {
		return "", fmt.Errorf("Clone URL field is not send in event")
	}
	home := fmt.Sprintf("/%s/%s/%s", basePath, owner, repository)

	if err := clone(fmt.Sprintf("%s", cloneURL), home); err != nil {
		return "", err
	}
	release := false
	if event.Type() == "release" {
		release = true
	}
	return home, checkout(home, event.ID(), release)
}

func checkoutKustomizationRepository(url, path string) error {
	return clone(url, path)
}

func clone(url, directory string) error {
	_, err := git.PlainClone(directory, false, &git.CloneOptions{
		URL:               url,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
	})
	return err
}

func checkout(directory, id string, isTag bool) error {
	r, err := git.PlainOpen(directory)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	if err := w.Pull(&git.PullOptions{RemoteName: "origin"}); err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	tags, err := r.Tags()
	if err != nil {
		return err
	}

	if isTag {
		if err := tags.ForEach(func(t *plumbing.Reference) error {
			if t.Name().String() == id {
				id = t.Hash().String()
				return nil
			}
			return fmt.Errorf("tag %q not found", id)
		}); err != nil {
			return err
		}
	}

	return w.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(id),
	})
}

func push(path string) error {
	r, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	_, err = w.Add("output.yaml")
	if err != nil {
		return err
	}

	if _, err := w.Commit("adding output.yaml", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "kustomizer",
			Email: "kustomizer@triggermesh.io",
			When:  time.Now(),
		},
	}); err != nil {
		return err
	}

	return r.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: "kustomizer",
			Password: cfg.Token,
		},
	})
}
