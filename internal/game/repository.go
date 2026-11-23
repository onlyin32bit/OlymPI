package game

import "context"

type Repository interface {
	ListMatches(context.Context, string) ([]Match, error)
	GetMatch(context.Context, string) (Match, error)
	CreateMatch(context.Context, Match, Actor) (Match, error)
	UpdateMatch(context.Context, Match, int64, Actor) (Match, error)
	SetMatchStatus(context.Context, string, MatchStatus, int64, Actor) (Match, error)
	ArchiveMatch(context.Context, string, int64, Actor) error
	SelectedMatch(context.Context) (string, int64, error)
	SelectMatch(context.Context, string, Actor) (int64, error)
	SaveRuntime(context.Context, RuntimeState, string, any, Actor, string) (RuntimeState, error)
	CompleteMatch(context.Context, RuntimeState, string, any, []byte, Actor, string) (RuntimeState, error)
	LoadRuntime(context.Context, string, int64) (RuntimeState, error)
	ListEvents(context.Context, string) ([]Event, error)
	ListEventsAfter(context.Context, string, int64) ([]Event, error)
	FinalizeResult(context.Context, string, []byte, Actor) error
	AddAmendment(context.Context, string, []byte, string, Actor) error

	ListQuestionSets(context.Context, string) ([]QuestionSet, error)
	GetQuestionSet(context.Context, string) (QuestionSet, QuestionSetRevision, error)
	GetQuestionSetRevision(context.Context, string) (QuestionSetRevision, error)
	CreateQuestionSet(context.Context, QuestionSet, QuestionSetRevision, Actor) (QuestionSet, error)
	SaveQuestionSet(context.Context, QuestionSet, QuestionSetRevision, int64, Actor) (QuestionSet, error)
	ArchiveQuestionSet(context.Context, string, int64, Actor) error
	PinSelectedMatchRevision(context.Context, string, string, string, Actor) (bool, error)

	ListAudit(context.Context, string, int) ([]AuditEntry, error)
	AppendAudit(context.Context, AuditEntry, Actor) error
	SaveAsset(context.Context, Asset, Actor) (Asset, error)
	DeleteAsset(context.Context, string) error
	GetAsset(context.Context, string) (Asset, error)
}
