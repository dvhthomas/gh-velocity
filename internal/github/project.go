package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ProjectInfo holds the resolved IDs needed for project board API calls.
type ProjectInfo struct {
	ProjectID     string // PVT_... node ID
	StatusFieldID string // PVTSSF_... field ID
}

// ParseProjectURL extracts the owner, project number, and owner type from a GitHub project URL.
// Accepts:
//   - https://github.com/users/{user}/projects/{N}
//   - https://github.com/orgs/{org}/projects/{N}
func ParseProjectURL(rawURL string) (owner string, number int, isOrg bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, false, fmt.Errorf("invalid project URL: %w", err)
	}
	if u.Host != "github.com" {
		return "", 0, false, fmt.Errorf("project URL must be a github.com URL, got host %q", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Strip trailing /views/{N} if present (browser URLs include the view).
	if len(parts) >= 6 && parts[4] == "views" {
		parts = parts[:4]
	}
	// Expected: users/{user}/projects/{N} or orgs/{org}/projects/{N}
	if len(parts) != 4 || (parts[0] != "users" && parts[0] != "orgs") || parts[2] != "projects" {
		return "", 0, false, fmt.Errorf("project URL must be https://github.com/users/{user}/projects/{N} or https://github.com/orgs/{org}/projects/{N}, got %q", rawURL)
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, false, fmt.Errorf("project URL must end with a project number, got %q", parts[3])
	}
	return parts[1], n, parts[0] == "orgs", nil
}

// ResolveProject resolves a GitHub project URL and status field name to their internal IDs.
// It fetches the project by number via GraphQL, then finds the named status field.
// For org/user project URLs, it queries the organization or user directly rather than
// going through the repository.
func (c *Client) ResolveProject(ctx context.Context, projectURL, statusFieldName string) (*ProjectInfo, error) {
	owner, number, isOrg, err := ParseProjectURL(projectURL)
	if err != nil {
		return nil, err
	}

	project, err := c.DiscoverProjectByOwner(ctx, owner, number, isOrg)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}

	info := &ProjectInfo{
		ProjectID: project.ID,
	}

	if statusFieldName == "" {
		return info, nil
	}

	for _, f := range project.Fields {
		if strings.EqualFold(f.Name, statusFieldName) && len(f.Options) > 0 {
			info.StatusFieldID = f.ID
			return info, nil
		}
	}

	return nil, fmt.Errorf("resolve project: status field %q not found on project %q (#%d)", statusFieldName, project.Title, project.Number)
}
