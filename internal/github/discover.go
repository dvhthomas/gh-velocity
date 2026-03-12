package github

import (
	"context"
	"fmt"
)

// DiscoveredProject represents a Projects v2 board linked to a repository.
type DiscoveredProject struct {
	ID     string            `json:"id"` // PVT_... node ID
	Title  string            `json:"title"`
	Number int               `json:"number"`
	URL    string            `json:"url"` // e.g. https://github.com/users/foo/projects/1
	Fields []DiscoveredField `json:"fields"`
}

// DiscoveredField represents a project field (e.g., Status).
type DiscoveredField struct {
	ID      string             `json:"id"` // PVTSSF_... for single-select
	Name    string             `json:"name"`
	Type    string             `json:"type"` // "ProjectV2SingleSelectField", etc.
	Options []DiscoveredOption `json:"options,omitempty"`
}

// DiscoveredOption represents an option within a single-select field.
type DiscoveredOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type discoverResponse struct {
	Repository struct {
		ProjectsV2 struct {
			Nodes []discoverProjectNode `json:"nodes"`
		} `json:"projectsV2"`
	} `json:"repository"`
}

type discoverProjectNode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Fields struct {
		Nodes []discoverFieldNode `json:"nodes"`
	} `json:"fields"`
}

type discoverFieldNode struct {
	Typename string             `json:"__typename"`
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Options  []DiscoveredOption `json:"options,omitempty"`
}

// DiscoverProjects lists Projects v2 boards linked to the repository,
// including their fields and single-select options.
func (c *Client) DiscoverProjects(ctx context.Context) ([]DiscoveredProject, error) {
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			projectsV2(first: 20) {
				nodes {
					id
					title
					number
					url
					fields(first: 30) {
						nodes {
							... on ProjectV2SingleSelectField {
								__typename
								id
								name
								options {
									id
									name
								}
							}
							... on ProjectV2Field {
								__typename
								id
								name
							}
							... on ProjectV2IterationField {
								__typename
								id
								name
							}
						}
					}
				}
			}
		}
	}`

	variables := map[string]any{
		"owner": c.owner,
		"repo":  c.repo,
	}

	var resp discoverResponse
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("discover projects for %s/%s: %w", c.owner, c.repo, err)
	}

	var projects []DiscoveredProject
	for _, p := range resp.Repository.ProjectsV2.Nodes {
		proj := DiscoveredProject{
			ID:     p.ID,
			Title:  p.Title,
			Number: p.Number,
			URL:    p.URL,
		}
		for _, f := range p.Fields.Nodes {
			field := DiscoveredField{
				ID:   f.ID,
				Name: f.Name,
				Type: f.Typename,
			}
			if len(f.Options) > 0 {
				field.Options = f.Options
			}
			proj.Fields = append(proj.Fields, field)
		}
		projects = append(projects, proj)
	}

	return projects, nil
}

// DiscoverProjectByNumber fetches a single Projects v2 board by its number,
// including its fields and single-select options.
func (c *Client) DiscoverProjectByNumber(ctx context.Context, number int) (*DiscoveredProject, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			projectV2(number: $number) {
				id
				title
				number
				url
				fields(first: 30) {
					nodes {
						... on ProjectV2SingleSelectField {
							__typename
							id
							name
							options {
								id
								name
							}
						}
						... on ProjectV2Field {
							__typename
							id
							name
						}
						... on ProjectV2IterationField {
							__typename
							id
							name
						}
					}
				}
			}
		}
	}`

	variables := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}

	var resp struct {
		Repository struct {
			ProjectV2 *discoverProjectNode `json:"projectV2"`
		} `json:"repository"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("discover project #%d for %s/%s: %w\n  hint: set GH_VELOCITY_TOKEN to a PAT with 'project' scope (see docs/guide.md#token-permissions)", number, c.owner, c.repo, err)
	}

	p := resp.Repository.ProjectV2
	if p == nil {
		return nil, fmt.Errorf("project #%d not found on %s/%s", number, c.owner, c.repo)
	}

	proj := &DiscoveredProject{
		ID:     p.ID,
		Title:  p.Title,
		Number: p.Number,
		URL:    p.URL,
	}
	for _, f := range p.Fields.Nodes {
		field := DiscoveredField{
			ID:   f.ID,
			Name: f.Name,
			Type: f.Typename,
		}
		if len(f.Options) > 0 {
			field.Options = f.Options
		}
		proj.Fields = append(proj.Fields, field)
	}

	return proj, nil
}
