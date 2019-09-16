package main

import (
	"context"
	"fmt"
	"log"

	cloudevents "github.com/cloudevents/sdk-go"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"knative.dev/eventing-contrib/pkg/kncloudevents"
)

/*
Example Output:

☁  cloudevents.Event:
Validation: valid
Context Attributes,
  SpecVersion: 0.2
  Type: dev.knative.eventing.samples.heartbeat
  Source: https://knative.dev/eventing-contrib/cmd/heartbeats/#local/demo
  ID: 3d2b5a1f-10ca-437b-a374-9c49e43c02fb
  Time: 2019-03-14T21:21:29.366002Z
  ContentType: application/json
  Extensions:
    the: 42
    beats: true
    heart: yes
Transport Context,
  URI: /
  Host: localhost:8080
  Method: POST
Data,
  {
    "id":162,
    "label":""
  }
*/

func main() {
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

	meta := event.Extensions()
	owner, exist := meta["owner"]
	if !exist {
		return fmt.Errorf("Owner field is not send in event")
	}
	repository, exist := meta["repository"]
	if !exist {
		return fmt.Errorf("Repository field is not send in event")
	}
	cloneURL, exist := meta["clone_url"]
	if !exist {
		return fmt.Errorf("Clone URL field is not send in event")
	}
	home := fmt.Sprintf("/tmp/%s/%s", owner, repository)

	return clone(fmt.Sprintf("%s", cloneURL), home)
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

	if err := w.Pull(&git.PullOptions{RemoteName: "origin"}); err != nil {
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
