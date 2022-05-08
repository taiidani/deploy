package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"text/template"

	"github.com/google/go-github/v44/github"
	"github.com/hashicorp/nomad/api"
)

type NomadDeployer struct {
	gh     *github.Client
	client *api.Client
}

type NomadDeployment struct {
	deployer   *NomadDeployer
	deployment *github.Deployment
	owner      string
	repo       string
}

const nomadJobPath = "nomad.job"

func NewNomadDeployer(gh *github.Client) *NomadDeployer {
	ret := &NomadDeployer{gh: gh}

	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		log.Fatal("Could not connect to Nomad")
	}
	ret.client = client

	return ret
}

func (n *NomadDeployer) Deploy(ctx context.Context, request *github.DeploymentEvent) (err error) {
	d := NomadDeployment{
		deployer:   n,
		deployment: request.Deployment,
		owner:      *request.Repo.Owner.Name,
		repo:       *request.Repo.Name,
	}

	defer func() {
		if err != nil {
			_ = d.UpdateStatus(ctx, &github.DeploymentStatusRequest{
				State:       github.String("error"),
				Description: github.String(err.Error()),
			})
		}
	}()

	if err := d.UpdateStatus(ctx, &github.DeploymentStatusRequest{
		State:       github.String("queued"),
		Description: github.String("job claimed"),
	}); err != nil {
		return err
	}

	job, err := n.getJob(ctx, request)
	if err != nil {
		return err
	}

	if resp, _, err := n.client.Jobs().Validate(job, &api.WriteOptions{}); err != nil {
		return fmt.Errorf("could not validate %s Nomad job: %w", nomadJobPath, err)
	} else if len(resp.ValidationErrors) > 0 {
		return fmt.Errorf("nomad job %s contained validation errors:\n%s", nomadJobPath, strings.Join(resp.ValidationErrors, "\n"))
	}

	if plan, _, err := n.client.Jobs().Plan(job, false, &api.WriteOptions{}); err != nil {
		return fmt.Errorf("could not plan %s Nomad job: %w", nomadJobPath, err)
	} else if len(plan.Warnings) > 0 {
		return fmt.Errorf("nomad job %s contained plan warnings: %s", nomadJobPath, plan.Warnings)
	}

	if err := d.UpdateStatus(ctx, &github.DeploymentStatusRequest{
		State:       github.String("in_progress"),
		Description: github.String("job deploying"),
	}); err != nil {
		return err
	}

	if run, _, err := n.client.Jobs().Register(job, &api.WriteOptions{}); err != nil {
		return fmt.Errorf("could not run %s Nomad job: %w", nomadJobPath, err)
	} else if len(run.Warnings) > 0 {
		return fmt.Errorf("nomad job %s contained run warnings: %s", nomadJobPath, run.Warnings)
	}

	if err := d.UpdateStatus(ctx, &github.DeploymentStatusRequest{
		State:       github.String("success"),
		Description: github.String("job deployed"),
	}); err != nil {
		return err
	}

	return nil
}

func (d *NomadDeployment) UpdateStatus(ctx context.Context, status *github.DeploymentStatusRequest) error {
	_, _, err := d.deployer.gh.Repositories.CreateDeploymentStatus(ctx, d.owner, d.repo, *d.deployment.ID, status)
	return err
}

func (n *NomadDeployer) getJob(ctx context.Context, request *github.DeploymentEvent) (*api.Job, error) {
	jobFile, _, _, err := n.gh.Repositories.GetContents(ctx,
		*request.Repo.Owner.Name,
		*request.Repo.Name,
		nomadJobPath,
		&github.RepositoryContentGetOptions{
			Ref: *request.Deployment.Ref,
		})
	if err != nil {
		return nil, fmt.Errorf("could not download %s file from %s: %w", nomadJobPath, *request.Repo.Name, err)
	}

	jobContent, err := jobFile.GetContent()
	if err != nil {
		return nil, fmt.Errorf("could not decode %s file: %w", nomadJobPath, err)
	}

	renderedJob, err := n.renderJob(jobContent, request)
	if err != nil {
		return nil, err
	}

	job, err := n.client.Jobs().ParseHCL(renderedJob, true)
	if err != nil {
		return job, fmt.Errorf("could not parse %s file as Nomad job: %w", nomadJobPath, err)
	}

	return job, nil
}

func (n *NomadDeployer) renderJob(job string, request *github.DeploymentEvent) (string, error) {
	tmpl, err := template.New("nomad-job").Parse(job)
	if err != nil {
		return "", fmt.Errorf("could not parse %s file as template: %w", nomadJobPath, err)
	}

	type templateParams struct {
		Request *github.DeploymentEvent
		Payload map[string]string
	}
	params := templateParams{Request: request}
	if err := json.NewDecoder(bytes.NewReader(request.Deployment.Payload)).Decode(&params.Payload); err != nil {
		return "", fmt.Errorf("could not unmarshal %s deployment payload: %w", nomadJobPath, err)
	}

	renderedJob := strings.Builder{}
	if err := tmpl.Execute(&renderedJob, params); err != nil {
		return "", fmt.Errorf("could not render %s template: %w", nomadJobPath, err)
	}

	return renderedJob.String(), nil
}
