package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TeamStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Audit        auditing.TransactionRunner
	Capabilities setupCapability
}

var _ app.TeamRepository = (*TeamStore)(nil)
var _ app.TeamLookup = (*TeamStore)(nil)

func TeamAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "org.team.changed", TargetType: "team", Reasons: []string{"created", "metadata", "archived", "restored", "member_added", "member_removed"}, RequireReason: true}}
}
func (s *TeamStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && s.Capabilities != nil
}
func (s *TeamStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, write bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, target, app.StructurePolicy(write))
}
func teamError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{app.ErrTeamNameDuplicate, d.ErrStructureInUse, d.ErrConditions, d.ErrConflict, d.ErrTransition, d.ErrRevision} {
		if errors.Is(err, known) {
			return err
		}
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P8001":
			return app.ErrTeamNameDuplicate
		case "P8002":
			return d.ErrConflict
		case "P8003":
			return d.ErrInvalid
		case "P8004", "P8006":
			return app.ErrUnavailable
		case "P8005":
			return d.ErrConditions
		case "P8007":
			return d.ErrStructureInUse
		case "23505":
			if pe.ConstraintName == "org_team_active_name_unique" {
				return app.ErrTeamNameDuplicate
			}
			return app.ErrUnavailable
		}
	}
	return legalError(ctx, err)
}
func team(scope kernel.OrgScope, row q.GetTeamRow) (d.Team, error) {
	c, err := d.RehydrateTeam(scope, d.TeamSnapshot{OrgID: scope.OrgID(), ID: row.ID, TeamProfile: d.TeamProfile{Name: row.Name, UnitID: row.UnitID}, State: d.TeamState(row.State), Revision: row.Revision})
	if err != nil {
		return d.Team{}, app.ErrUnavailable
	}
	return c, nil
}
func loadTeam(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.Team, error) {
	row, err := q.New(tx).GetTeam(ctx, q.GetTeamParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.Team{}, err
	}
	return team(scope, row)
}
func (s *TeamStore) FindTeam(ctx context.Context, scope kernel.OrgScope, id string) (d.Team, error) {
	if err := legalIdentity(scope, id); err != nil {
		return d.Team{}, err
	}
	if s == nil || s.Pool == nil {
		return d.Team{}, app.ErrUnavailable
	}
	var result d.Team
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error { var err error; result, err = loadTeam(ctx, tx, scope, id); return err })
	if err != nil {
		return d.Team{}, teamError(ctx, err)
	}
	return result, nil
}
func (s *TeamStore) WriteTeam(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, newID string, command app.TeamWrite) (app.TeamResult, bool, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.TeamResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.TeamResult{}, false, err
	}
	if (v.Kind == app.CreateTeam || v.Kind == app.AddTeamEmployee) && legalIdentity(scope, newID) != nil || v.Kind != app.CreateTeam && v.Kind != app.AddTeamEmployee && newID != "" {
		return app.TeamResult{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return app.TeamResult{}, false, app.ErrUnavailable
	}
	target := v.TeamID
	if v.Kind == app.CreateTeam {
		target = scope.OrgID().String()
	}
	digest := command.Digest(scope)
	var result app.TeamResult
	replay := false
	readyToCommit := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, target, true); err != nil {
			return err
		}
		queries := q.New(tx)
		prior, err := queries.TeamCommandReceipt(ctx, q.TeamCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			if legalIdentity(scope, prior.TeamID) != nil || prior.ResultRevision < 1 || prior.ResultState != "ACTIVE" && prior.ResultState != "ARCHIVED" {
				return app.ErrUnavailable
			}
			if v.Kind == app.AddTeamEmployee || v.Kind == app.RemoveTeamEmployee {
				if legalIdentity(scope, prior.LinkID) != nil {
					return app.ErrUnavailable
				}
			} else if prior.LinkID != "" {
				return app.ErrUnavailable
			}

			result = app.TeamResult{TeamID: prior.TeamID, State: d.TeamState(prior.ResultState), Revision: prior.ResultRevision, LinkID: prior.LinkID}
			replay = true
			readyToCommit = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		eventTarget, reason := v.TeamID, "metadata"
		switch v.Kind {
		case app.CreateTeam:
			eventTarget, reason = newID, "created"
		case app.ArchiveTeam:
			reason = "archived"
		case app.RestoreTeam:
			reason = "restored"
		case app.AddTeamEmployee:
			reason = "member_added"
		case app.RemoveTeamEmployee:
			reason = "member_removed"
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: "org.team.changed", TargetType: "team", TargetID: eventTarget, RequestHash: digest, ReasonCode: reason}, func(ctx context.Context, tx pgx.Tx) error {
			var updated d.Team
			linkID := ""
			var err error
			if v.Kind == app.CreateTeam {
				updated, err = d.NewTeam(scope, newID, v.Profile)
			} else {
				current, readErr := loadTeam(ctx, tx, scope, v.TeamID)
				if readErr != nil {
					return readErr
				}
				before, readErr := current.Snapshot(scope)
				if readErr != nil {
					return readErr
				}
				if before.Revision != v.ExpectedRevision {
					return d.ErrConflict
				}
				switch v.Kind {
				case app.ChangeTeam:
					updated, err = current.Change(scope, v.ExpectedRevision, v.Profile)
				case app.AddTeamEmployee, app.RemoveTeamEmployee:
					employee, readErr := s.Memberships.GetEmployeeTx(ctx, tx, scope, v.EmployeeID)
					if errors.Is(readErr, members.ErrNotFound) {
						return app.ErrNotFound
					}
					if readErr != nil {
						return app.ErrUnavailable
					}
					employeeSnapshot, readErr := employee.Snapshot(scope)
					if readErr != nil {
						return app.ErrUnavailable
					}
					updated, err = current.MemberChange(scope, v.ExpectedRevision, d.TeamEmployee{OrgID: scope.OrgID(), ID: employeeSnapshot.ID, Active: employeeSnapshot.State == "ACTIVE"}, v.Kind == app.AddTeamEmployee)
					if err == nil {
						if v.Kind == app.AddTeamEmployee {
							linkID = newID
						} else {
							link, readErr := queries.CurrentTeamLink(ctx, q.CurrentTeamLinkParams{OrgID: scope.OrgID().String(), TeamID: v.TeamID, EmployeeID: v.EmployeeID})
							if errors.Is(readErr, pgx.ErrNoRows) {
								return d.ErrConflict
							}
							if readErr != nil {
								return readErr
							}
							linkID = link.ID
						}
					}

				case app.RestoreTeam:
					updated, err = current.Restore(scope, v.ExpectedRevision)
				case app.ArchiveTeam:
					if before.State != d.TeamActive {
						return d.ErrTransition
					}
					raw, readErr := queries.TeamCurrentUsage(ctx, q.TeamCurrentUsageParams{OrgID: scope.OrgID().String(), ID: v.TeamID})
					if readErr != nil {
						return readErr
					}
					usage, readErr := decodeUnitUsage(raw)
					if readErr != nil {
						return readErr
					}
					updated, err = current.Archive(scope, v.ExpectedRevision, usage)
					if errors.Is(err, d.ErrStructureInUse) {
						return app.StructureInUse(usage)
					}
				default:
					return d.ErrInvalid
				}
			}
			if err != nil {
				return err
			}
			snapshot, err := updated.Snapshot(scope)
			if err != nil {
				return err
			}
			if v.Kind == app.CreateTeam {
				err = queries.InsertTeam(ctx, q.InsertTeamParams{OrgID: scope.OrgID().String(), ID: snapshot.ID, Name: snapshot.Name, UnitID: snapshot.UnitID})
			} else if v.Kind == app.AddTeamEmployee {
				err = queries.InsertTeamLink(ctx, q.InsertTeamLinkParams{OrgID: scope.OrgID().String(), ID: linkID, TeamID: v.TeamID, EmployeeID: v.EmployeeID, OperationID: v.OperationID})
			} else if v.Kind == app.RemoveTeamEmployee {
				changed, writeErr := queries.EndTeamLink(ctx, q.EndTeamLinkParams{OrgID: scope.OrgID().String(), ID: linkID, OperationID: v.OperationID})
				err = writeErr
				if err == nil && changed != 1 {
					return d.ErrConflict
				}

			} else {
				changed, writeErr := queries.ChangeTeam(ctx, q.ChangeTeamParams{OrgID: scope.OrgID().String(), ID: snapshot.ID, ExpectedRevision: v.ExpectedRevision, Name: snapshot.Name, UnitID: snapshot.UnitID, State: string(snapshot.State)})
				err = writeErr
				if err == nil && changed != 1 {
					return d.ErrConflict
				}
			}
			if err != nil {
				return err
			}
			actual, readErr := loadTeam(ctx, tx, scope, snapshot.ID)
			if readErr != nil {
				return readErr
			}
			actualSnapshot, readErr := actual.Snapshot(scope)
			if readErr != nil || actualSnapshot != snapshot {
				return app.ErrUnavailable
			}

			result = app.TeamResult{TeamID: snapshot.ID, State: snapshot.State, Revision: snapshot.Revision, LinkID: linkID}
			return queries.InsertTeamReceipt(ctx, q.InsertTeamReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: actor.AccountID, TeamID: snapshot.ID, LinkID: linkID, Kind: string(v.Kind), RequestHash: digest, ResultState: string(snapshot.State), ResultRevision: snapshot.Revision})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		readyToCommit = true
		return nil
	})
	if err != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.TeamResult{}, false, app.ErrUnavailable
		}
		return app.TeamResult{}, false, teamError(ctx, err)
	}
	return result, replay, nil
}
func (s *TeamStore) ReadTeam(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string) (d.Team, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return d.Team{}, err
	}
	if err := legalIdentity(scope, id); err != nil {
		return d.Team{}, err
	}
	if !s.ready() {
		return d.Team{}, app.ErrUnavailable
	}
	var result d.Team
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, false); err != nil {
			return err
		}
		var err error
		result, err = loadTeam(ctx, tx, scope, id)
		return err
	})
	if err != nil {
		return d.Team{}, teamError(ctx, err)
	}
	return result, nil
}
func (s *TeamStore) ListTeams(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, filter app.TeamFilter) (app.TeamPage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.TeamPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.TeamPage{}, err
	}
	if !s.ready() {
		return app.TeamPage{}, app.ErrUnavailable
	}
	result := app.TeamPage{Teams: []d.Team{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, scope.OrgID().String(), false); err != nil {
			return err
		}
		rows, err := q.New(tx).ListTeams(ctx, q.ListTeamsParams{OrgID: scope.OrgID().String(), AfterID: filter.AfterID, StateFilter: string(filter.State), RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				last, err := result.Teams[len(result.Teams)-1].Snapshot(scope)
				if err != nil {
					return err
				}
				result.NextID = last.ID
				break
			}
			value, err := team(scope, q.GetTeamRow(row))
			if err != nil {
				return err
			}
			result.Teams = append(result.Teams, value)
		}
		return nil
	})
	if err != nil {
		return app.TeamPage{}, teamError(ctx, err)
	}
	return result, nil
}

func teamLink(scope kernel.OrgScope, row q.CurrentTeamLinkRow) (d.TeamLink, error) {
	if !row.StartedAt.Valid || row.StartedAt.InfinityModifier != pgtype.Finite || row.EndedAt.Valid && row.EndedAt.InfinityModifier != pgtype.Finite {
		return d.TeamLink{}, app.ErrUnavailable
	}
	snap := d.TeamLinkSnapshot{OrgID: scope.OrgID(), ID: row.ID, TeamID: row.TeamID, EmployeeID: row.EmployeeID, StartedAt: row.StartedAt.Time, EndReason: d.TeamLinkEndReason(row.EndReason)}
	if row.EndedAt.Valid {
		snap.EndedAt = row.EndedAt.Time
	}
	link, err := d.RehydrateTeamLink(scope, snap)
	if err != nil {
		return d.TeamLink{}, app.ErrUnavailable
	}
	return link, nil
}
func (s *TeamStore) ListTeamLinks(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string, f app.TeamLinkFilter) (app.TeamLinkPage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.TeamLinkPage{}, err
	}
	if legalIdentity(scope, id) != nil || f.Validate() != nil {
		return app.TeamLinkPage{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.TeamLinkPage{}, app.ErrUnavailable
	}
	result := app.TeamLinkPage{Links: []d.TeamLink{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, false); err != nil {
			return err
		}
		if _, err := loadTeam(ctx, tx, scope, id); err != nil {
			return err
		}
		rows, err := q.New(tx).ListTeamLinks(ctx, q.ListTeamLinksParams{OrgID: scope.OrgID().String(), TeamID: id, StateFilter: f.State, AfterID: f.AfterID, RowLimit: int32(f.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == f.Limit {
				last, _ := result.Links[i-1].Snapshot(scope)
				result.NextID = last.ID
				break
			}
			link, err := teamLink(scope, q.CurrentTeamLinkRow(row))
			if err != nil {
				return err
			}
			result.Links = append(result.Links, link)
		}
		return nil
	})
	if err != nil {
		return app.TeamLinkPage{}, teamError(ctx, err)
	}
	return result, nil
}
