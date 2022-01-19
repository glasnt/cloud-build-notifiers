// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	webhookURLSecretName = "webhookUrl"
)

func main() {
	if err := notifiers.Main(new(googlechatNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type googlechatNotifier struct {
	filter notifiers.EventFilter

	webhookURL string
}

func (g *googlechatNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	g.filter = prd

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, webhookURLSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, webhookURLSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %w", err)
	}
	g.webhookURL = wu

	return nil
}

func (g *googlechatNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !g.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Google Chat webhook for Build %q (status: %q)", build.Id, build.Status)
	msg, err := g.writeMessage(build)
	if err != nil {
		return fmt.Errorf("failed to write Google Chat message: %w", err)
	}

	payload := new(bytes.Buffer)
	json.NewEncoder(payload).Encode(msg)

	// TODO(glasnt) adjust
	//return slack.PostWebhook(s.webhookURL, msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.webhookURL, payload)
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GCB-Notifier/0.1 (http)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warningf("got a non-OK response status %q (%d) from %q", resp.Status, resp.StatusCode, g.webhookURL)
		log.Warningf("resp: %s", resp.Body)
	}

	log.V(2).Infoln("send HTTP request successfully")
	return nil
}

type ChatMessage struct { 
	Text string `json:"text"`
}

//func (g *googlechatNotifier) writeMessage(build *cbpb.Build) (*chat.Message, error) {
func (g *googlechatNotifier) writeMessage(build *cbpb.Build) (*ChatMessage, error) {

	var clr string

	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "good"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "danger"
	default:
		clr = "warning"
	}

	txt := fmt.Sprintf(
		"v0.2 Cloud Build %s (%s, %s): %s",
		clr,
		build.ProjectId,
		build.Id,
		build.Status,
	)

	msg := ChatMessage{Text: txt}

	//var jsonStr = []byte(fmt.Sprintf(`{"text": "%s"}`, txt))
	//log.Warningf("jsonStr: %s", jsonStr)
	// TODO(glasnt) adjust
	return &msg, nil

}
