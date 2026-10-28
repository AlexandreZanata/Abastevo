// Package seeds embeds the explicit dev/staging seed scripts. They are never
// part of the migration chain; operators run them on purpose.
package seeds

import _ "embed"

// ProjectCreator is the idempotent seed for the project creator contributor.
//
//go:embed project_creator.sql
var ProjectCreator string
