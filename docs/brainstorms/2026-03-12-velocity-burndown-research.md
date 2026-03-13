# Velocity, Burndown, and Burnup: Deep Research Findings

Date: 2026-03-12

---

## 1. GitHub Projects v2 Built-in Charts

### What Exists Natively

GitHub Projects v2 provides an **Insights** tab with chart capabilities:

- **Default chart**: "Burn up" (not burndown) -- visualizes progress of issues over time
- **Historical charts**: X-axis is time, tracking item state changes
- **Current charts**: Snapshot of current state (e.g., stacked column by iteration/category)

### How "Done" Is Defined

GitHub tracks four item states in historical charts:
1. **Open** -- open issues and open PRs
2. **Completed** -- issues closed as "completed" (`reason:completed`) OR merged PRs
3. **Closed pull requests** -- PRs closed without merge
4. **Not planned** -- issues closed as "not planned" (`reason:"not planned"`)

Key insight: GitHub distinguishes `reason:completed` from `reason:"not planned"` at the search API level. This is queryable via `reason:completed` qualifier.

### Chart Limitations

- **No native burndown chart** -- only burn-up
- **No native velocity chart** -- this is a longstanding feature request (community discussion #38840, open since Nov 2022, no GitHub staff response with a roadmap)
- **No story point aggregation** in charts -- charts count items, not points
- Insights does **not** track archived or deleted items
- Historical data is lost if items are removed

### Mid-Sprint Scope Changes

The burn-up chart shows scope changes as small circles in the timeline when items are added or removed during an iteration. However, detailed scope-change tracking is minimal.

### What Data Is Available via API

All chart data must be reconstructed from the GraphQL API -- there is no "insights data" API endpoint. You query project items with their field values and compute metrics yourself.

---

## 2. GitHub Projects v2 Iteration Field API (Verified via GraphQL Introspection)

### ProjectV2IterationField

Top-level type for an iteration field on a project.

```graphql
type ProjectV2IterationField {
  configuration: ProjectV2IterationFieldConfiguration!  # iteration config
  createdAt: DateTime!
  dataType: ProjectV2FieldType!    # always ITERATION
  databaseId: Int
  id: ID!
  name: String!
  project: ProjectV2!
  updatedAt: DateTime!
}
```

### ProjectV2IterationFieldConfiguration

Contains both active and completed iterations:

```graphql
type ProjectV2IterationFieldConfiguration {
  completedIterations: [ProjectV2IterationFieldIteration!]!  # past/completed
  duration: Int!           # default duration in days for new iterations
  iterations: [ProjectV2IterationFieldIteration!]!           # active/upcoming
  startDay: Int!           # day of week iterations start (0=Sunday?)
}
```

**Key finding**: `completedIterations` and `iterations` are separate arrays. Past iterations move to `completedIterations`. Both contain the same type.

### ProjectV2IterationFieldIteration

Individual iteration definition (used in both `iterations` and `completedIterations`):

```graphql
type ProjectV2IterationFieldIteration {
  duration: Int!      # duration in days
  id: String!         # unique iteration ID (used for mutations)
  startDate: Date!    # YYYY-MM-DD format
  title: String!      # display name (e.g., "Sprint 5")
  titleHTML: String!   # HTML-rendered title
}
```

**End date** is computed: `endDate = startDate + duration days`.

### ProjectV2ItemFieldIterationValue

The value on a project item indicating which iteration it belongs to:

```graphql
type ProjectV2ItemFieldIterationValue {
  createdAt: DateTime!
  creator: Actor
  databaseId: Int
  duration: Int!          # duration of the assigned iteration
  field: ProjectV2FieldConfiguration!
  id: ID!
  item: ProjectV2Item!
  iterationId: String!    # matches ProjectV2IterationFieldIteration.id
  startDate: Date!        # start date of the assigned iteration
  title: String!          # iteration title
  titleHTML: String!
  updatedAt: DateTime!
}
```

### ProjectV2ItemFieldNumberValue (for Story Points)

Number fields (commonly used for story points/effort):

```graphql
type ProjectV2ItemFieldNumberValue {
  createdAt: DateTime!
  creator: Actor
  databaseId: Int
  field: ProjectV2FieldConfiguration!
  id: ID!
  item: ProjectV2Item!
  number: Float           # nullable -- the numeric value
  updatedAt: DateTime!
}
```

### Example Query: Items with Iteration and Points

```graphql
query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          content {
            ... on Issue {
              title
              state
              stateReason
              closedAt
              createdAt
            }
            ... on PullRequest {
              title
              state
              mergedAt
              closedAt
              createdAt
            }
          }
          sprint: fieldValueByName(name: "Sprint") {
            ... on ProjectV2ItemFieldIterationValue {
              iterationId
              title
              startDate
              duration
            }
          }
          points: fieldValueByName(name: "Points") {
            ... on ProjectV2ItemFieldNumberValue {
              number
            }
          }
          status: fieldValueByName(name: "Status") {
            ... on ProjectV2ItemFieldSingleSelectValue {
              name
            }
          }
        }
      }
    }
  }
}
```

### Example Query: Iteration Field Configuration

```graphql
query($projectId: ID!) {
  node(id: $projectId) {
    ... on ProjectV2 {
      fields(first: 20) {
        nodes {
          ... on ProjectV2IterationField {
            id
            name
            configuration {
              duration
              startDay
              iterations {
                id
                title
                startDate
                duration
              }
              completedIterations {
                id
                title
                startDate
                duration
              }
            }
          }
        }
      }
    }
  }
}
```

### Mutation: Assign Item to Iteration

```graphql
mutation {
  updateProjectV2ItemFieldValue(input: {
    projectId: "PROJECT_ID"
    itemId: "ITEM_ID"
    fieldId: "FIELD_ID"
    value: { iterationId: "ITERATION_ID" }
  }) {
    projectV2Item { id }
  }
}
```

### Project View Filtering for Iterations

Within project views, iteration filtering supports relative keywords:
- `iteration:@current` -- current iteration
- `iteration:@next` -- next iteration
- `iteration:@previous` -- previous iteration
- `iteration:@current..@current+3` -- range of iterations

---

## 3. GitHub Issues Search Qualifiers Relevant to Velocity

### Available Search Qualifiers

| Qualifier | Example | Notes |
|-----------|---------|-------|
| `label:"NAME"` | `label:"bug"` | Multiple labels = AND |
| `type:issue` / `type:pr` | `type:issue` | Also `is:issue`, `is:pr` |
| `milestone:"NAME"` | `milestone:"Sprint 5"` | By milestone name |
| `project:ORG/NUM` | `project:github/57` | By project number |
| `state:open` / `state:closed` | `state:closed` | Also `is:open`, `is:closed` |
| `reason:completed` | `reason:completed` | Issues closed as done |
| `reason:"not planned"` | `reason:"not planned"` | Issues closed as not planned |
| `no:label` | `no:milestone` | Missing metadata |

### Can You Search by Project Field Values via Search API?

**Within project views**: Yes -- `field:VALUE` syntax (e.g., `status:done`, `field.priority:high`).

**Via the Issues search API**: No direct support for project field values. The `project:` qualifier only filters by project membership, not by field values within the project. You cannot do `project.iteration:"Sprint 5"` in the search API.

**Issue fields (org-level, private preview)**: GitHub is introducing "issue fields" with `field:` search syntax at the repository level, with REST/GraphQL API support behind the `GraphQL-Features: issue_fields` header. These are distinct from project fields.

**Practical implication for velocity**: To find items in a specific iteration, you must query the project items via GraphQL and filter client-side by iteration, or use project view filters. The search API cannot scope to iteration values.

---

## 4. Third-Party Tool Approaches to Velocity

### ZenHub

**Velocity definition**: Average story points completed per sprint over last 7 closed sprints.

**Formula**: `Velocity = Sum(story points closed per sprint) / Number of sprints`

**Key details**:
- Only fully completed items count toward velocity -- open/in-progress points are excluded
- Story points are assigned via ZenHub's built-in estimate field (not GitHub labels)
- "Completed" is configurable -- can be a pipeline (column) rather than just GitHub's "closed" state
- **Carry-over handling**: Issues can live in multiple sprints simultaneously. When a sprint ends, unclosed issues auto-move to next sprint. This means carry-over work is counted in the sprint where it actually closes, not where it was committed
- **Committed vs Completed**: Committed = all issues in the sprint at any point; Completed = issues that reached a "done" pipeline during the sprint
- Sprint boundaries = fixed time-boxes configured in ZenHub (not GitHub milestones)
- Velocity chart shows a bar per sprint + rolling average line

**Burndown calculation**: Plots remaining story points over time within a sprint. Ideal line is linear from total committed points to zero.

### Screenful (Analytics & Reports)

- Imports GitHub Projects custom fields directly (including iteration and number fields)
- Can create burndown charts scoped to iterations
- Supports velocity as points completed per iteration
- Uses GitHub Projects' own field values rather than labels

### Flow2C

- Provides burndown and burn-up for GitHub repos
- Tracks scope changes (items added/removed mid-sprint)
- Shows delivery pace trends

### Target/burndown-for-github-projects

- Open-source GitHub Action
- Story points via **digit-only issue labels** (e.g., label "5" = 5 points)
- Sprint boundaries defined by project name regex: `Sprint \d+ - (?<end_date>\d+/\d+/\d+)`
- Aggregates points by project column position
- No retroactive data -- only collects while running
- Uses GitHub REST API with `repo` and `read:org` scopes

### LinearB

- Focuses on DORA metrics and flow metrics over traditional velocity
- Defines Flow Velocity = number of flow items (features, defects, risks, debt) delivered per time period
- Explicitly warns against using velocity as a productivity metric
- Prefers: cycle time, deployment frequency, change failure rate, MTTR

### Swarmia

- Combines Jira/Linear sprint data with GitHub git activity
- Sprint metrics include committed vs completed
- DORA and SPACE framework metrics
- Does not rely on GitHub Projects for sprint boundaries

---

## 5. Common Velocity and Burndown Formulas

### Velocity

```
Velocity = Total Story Points Completed in Sprint

Average Velocity = Sum(Velocity per sprint) / Number of Sprints
```

- Only count **fully completed** items (not partially done)
- Use rolling average (typically 3-7 sprints) for planning
- Carry-over items count in the sprint where they are completed, not committed

### Completion Rate

```
Completion Rate = Story Points Completed / Story Points Committed

-- or by count:
Completion Rate = Items Completed / Items Committed
```

- "Committed" = items assigned to the sprint at sprint start (or at any point, depending on tool)
- "Completed" = items that reached a done state during the sprint
- Rates above 100% are possible if unplanned work is completed

### Burndown

```
Day N remaining = Total Committed Points - Sum(Points Completed through Day N)

Ideal Burndown Line:
  Day 0 = Total Committed Points
  Day D (end) = 0
  Day N = Total Committed - (Total Committed * N / D)
```

- Scope changes shift the "remaining" line up (items added) or down (items removed)
- Some tools recalculate the ideal line when scope changes; others keep the original ideal
- Mid-sprint additions shown as discontinuities or markers

### Burn-Up

```
Day N completed = Sum(Points Completed through Day N)
Day N total scope = Total Points in Sprint (may change over time)
```

- Two lines: "total scope" (may increase) and "completed" (monotonically increasing)
- Advantage over burndown: scope changes are visible as the total line moving up
- Sprint is on track when completed line approaches total scope line

### Carry-Over Handling

Three common approaches:
1. **Re-estimate**: Carry-over items get fresh estimates in the new sprint; original sprint velocity reflects only what was done
2. **Split**: Break carry-over items into "done" portion (counted in old sprint) and "remaining" (new sprint)
3. **Move whole**: Entire item moves to next sprint; counts as zero in the old sprint and full points in the new sprint (ZenHub's approach)

---

## 6. Practical Implications for gh-velocity

### Data Available Without Project Board

- Issue state and close reason (`reason:completed` vs `reason:"not planned"`)
- PR state (merged vs closed)
- Labels (can encode points as digit-only labels)
- Milestones (name, due date -- can serve as sprint boundaries)
- Timestamps (created, closed, merged)

### Data Requiring Project Board (GraphQL)

- Iteration field values (which sprint an item belongs to)
- Iteration boundaries (start date, duration, derived end date)
- Number field values (story points/effort as project custom fields)
- Single-select field values (status column)
- Historical state changes are NOT available via API -- only current state

### Key Limitation

**GitHub does not expose historical item-field-value changes via API.** You can see an item's current iteration and current points, but not when it was assigned to that iteration or when its points changed. This means:

- **True burndown** (remaining work plotted daily) requires periodic snapshots collected over time (like target/burndown-for-github-projects does via cron)
- **Velocity** can be computed from current data: query items in a completed iteration, sum points of those that are completed/merged
- **Completion rate** can be computed: completed items in iteration / total items in iteration
- **Burn-up** requires periodic snapshots for the time-series view, OR can be approximated using `closedAt`/`mergedAt` timestamps (when each item was completed)

### Recommended Approach for Velocity Command

1. Query project iteration field configuration to get iteration boundaries
2. Query all items assigned to a target iteration
3. Classify each item as completed (closed+completed / merged) or not
4. Sum points (from number field) for completed items = velocity
5. Sum points for all items = committed
6. Completion rate = velocity / committed
7. For historical velocity, repeat across `completedIterations`
8. For burndown approximation, use `closedAt` timestamps within the iteration date range

---

## Sources

- [GitHub Docs: About insights for Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/viewing-insights-from-your-project/about-insights-for-projects)
- [GitHub Docs: Using the API to manage Projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects)
- [GitHub Docs: Searching issues and pull requests](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
- [GitHub Docs: Filtering projects](https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/filtering-projects)
- [GitHub Community Discussion #38840: Burndown and Velocity charts](https://github.com/orgs/community/discussions/38840)
- [GitHub Community Discussion #157957: GraphQL iteration field mutations](https://github.com/orgs/community/discussions/157957)
- [GitHub Changelog: Issues advanced search API](https://github.blog/changelog/2025-03-06-github-issues-projects-api-support-for-issues-advanced-search-and-more/)
- [Some Natalie: GraphQL intro with custom fields](https://some-natalie.dev/blog/graphql-intro/)
- [JSR: ProjectV2IterationField type definition](https://jsr.io/@hk/github-graphql/doc/req/~/ProjectV2IterationField)
- [ZenHub: Track team velocity](https://help.zenhub.com/support/solutions/articles/43000010358-track-team-velocity-sprint-over-sprint)
- [ZenHub: How to measure velocity](https://www.zenhub.com/blog-posts/how-to-measure-team-velocity-and-meet-deadlines/)
- [ZenHub: Burndown and velocity complement each other](https://help.zenhub.com/support/solutions/articles/43000483134-how-burndown-and-velocity-compliment-each-other)
- [ZenHub: Introducing sprints](https://blog.zenhub.com/introducing-zenhub-sprints/)
- [ZenHub: Burndown charts for sprint tracking](https://blog.zenhub.com/tracking-sprint-progress-with-scrum-burndown-charts/)
- [LinearB: Why velocity is dangerous](https://linearb.io/blog/why-agile-velocity-is-the-most-dangerous-metric-for-software-development-teams)
- [LinearB: Flow metrics](https://linearb.io/blog/5-key-flow-metrics)
- [LinearB: SPACE framework](https://linearb.io/blog/space-framework)
- [Target: burndown-for-github-projects](https://github.com/target/burndown-for-github-projects)
- [Screenful: Charts with GitHub Projects custom fields](https://screenful.com/blog/create-advanced-charts-with-github-projects-custom-fields)
- [Agile Academy: Velocity definition](https://www.agile-academy.com/en/scrum-master/velocity-definition-and-how-you-can-calculate-it/)
- [Lucid: How to calculate sprint velocity](https://lucid.co/blog/how-to-calculate-sprint-velocity)
- [Swarmia: Sprint metrics](https://www.swarmia.com/product/sprints/)
