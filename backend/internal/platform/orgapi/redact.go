package orgapi

import "log/slog"

const redactedValue = "orgapi: private value redacted"

func (Session) String() string          { return redactedValue }
func (Session) GoString() string        { return redactedValue }
func (Session) LogValue() slog.Value    { return slog.StringValue(redactedValue) }
func (Command) String() string          { return redactedValue }
func (Command) GoString() string        { return redactedValue }
func (Command) LogValue() slog.Value    { return slog.StringValue(redactedValue) }
func (Actor) String() string            { return redactedValue }
func (Actor) GoString() string          { return redactedValue }
func (Actor) LogValue() slog.Value      { return slog.StringValue(redactedValue) }
func (Membership) String() string       { return redactedValue }
func (Membership) GoString() string     { return redactedValue }
func (Membership) LogValue() slog.Value { return slog.StringValue(redactedValue) }
func (Principal) String() string        { return redactedValue }
func (Principal) GoString() string      { return redactedValue }
func (Principal) LogValue() slog.Value  { return slog.StringValue(redactedValue) }
func (Page) String() string             { return redactedValue }
func (Page) GoString() string           { return redactedValue }
func (Page) LogValue() slog.Value       { return slog.StringValue(redactedValue) }
func (Gateway) String() string          { return redactedValue }
func (Gateway) GoString() string        { return redactedValue }
func (Gateway) LogValue() slog.Value    { return slog.StringValue(redactedValue) }
