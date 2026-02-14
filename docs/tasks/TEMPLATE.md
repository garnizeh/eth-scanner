# [Task ID]: [Task Title]

**Phase:** P0X - [Phase Name]  
**Status:** Not Started | In Progress | Completed | Blocked  
**Priority:** High | Medium | Low  
**Dependencies:** [List of task IDs or "None"]  
**Estimated Effort:** Small (< 1h) | Medium (1-3h) | Large (3h+)

---

## Description

[Clear, concise description of what this task accomplishes. Be specific about the deliverable.]

Example:
> Implement the SQLite database connection wrapper in `internal/database/db.go` using `modernc.org/sqlite` (pure Go, no CGO). The wrapper should handle connection initialization, schema application, and graceful shutdown.

---

## Acceptance Criteria

- [ ] Criterion 1: [Specific, testable outcome]
- [ ] Criterion 2: [Specific, testable outcome]
- [ ] Criterion 3: [Specific, testable outcome]

Example:
- [ ] `internal/database/db.go` file created with `DB` struct
- [ ] Connection function accepts database file path
- [ ] Schema from `docs/database/schema.sql` is applied on init
- [ ] Unit test verifies successful connection and table creation

---

## Implementation Notes

[Technical details, architectural decisions, code patterns to follow]

**Key Points:**
- Reference SDD section(s): [e.g., "Section 3.2: Database Design"]
- Libraries to use: [e.g., `modernc.org/sqlite`, `database/sql`]
- Code style: Follow `copilot-instructions.md` guidelines
- Performance considerations: [if applicable]

**Code Snippet (if helpful):**
```go
// Example structure
type DB struct {
    conn *sql.DB
}

func NewDB(path string) (*DB, error) {
    // implementation
}
```

---

## Testing

**How to verify this task is complete:**

1. **Unit Test:** [Describe test scenario]
   ```bash
   go test ./internal/database/... -v
   ```

2. **Manual Verification:** [If applicable]
   ```bash
   # Example command to test manually
   go run ./cmd/master
   ```

3. **Expected Outcome:** [What should happen when successful]

---

## References

- **SDD:** `docs/architecture/system-design-document.md` (Section X.Y)
- **Schema:** `docs/database/schema.sql`
- **Copilot Instructions:** `.github/copilot-instructions.md`
- **Related Tasks:** [List task IDs that are related or dependent]

---

## Progress Log

### [Date] - [Your Name/Initials]
- Started implementation
- [Note any blockers, decisions, or important findings]

### [Date] - [Your Name/Initials]
- Completed acceptance criteria 1-3
- Task moved to `done/`

---

## Notes

[Any additional context, gotchas, or things to remember for future reference]
