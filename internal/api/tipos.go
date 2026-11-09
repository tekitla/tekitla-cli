package api

import "time"

// Tipos del JSON que devuelve tekitla-gateway para issues.v1 (tekitla-protos
// v0.5.0, protojson: nombres en camelCase, enums como string, int32 como
// número). Solo los campos que usa tk; los demás se ignoran.

// Proyecto es issues.v1.Project.
type Proyecto struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	WorkspaceID string `json:"workspaceId"`
}

// Issue es issues.v1.Issue.
type Issue struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	Number          int       `json:"number"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	State           string    `json:"state"`
	Priority        string    `json:"priority"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	StatusID        string    `json:"statusId"`
	LabelIDs        []string  `json:"labelIds"`
	AssigneeActorID string    `json:"assigneeActorId"`
}

// Estado es issues.v1.Status (una columna del tablero).
type Estado struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Hidden   bool   `json:"hidden"`
}

// Etiqueta es issues.v1.Label.
type Etiqueta struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Comentario es issues.v1.Comment.
type Comentario struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId"`
	AuthorID  string    `json:"authorId"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Evento es issues.v1.IssueEvent. El oneof `change` llega como un solo campo
// presente (protojson omite los demás aunque EmitUnpopulated esté activo).
type Evento struct {
	ID              string           `json:"id"`
	ActorID         string           `json:"actorId"`
	AgentID         string           `json:"agentId"`
	CreatedAt       time.Time        `json:"createdAt"`
	Created         *struct{}        `json:"created"`
	StatusChanged   *CambioEstado    `json:"statusChanged"`
	TitleChanged    *CambioTexto     `json:"titleChanged"`
	DescChanged     *struct{}        `json:"descriptionChanged"`
	PriorityChanged *CambioTexto     `json:"priorityChanged"`
	LabelsChanged   *CambioEtiquetas `json:"labelsChanged"`
	Commented       *Comentario      `json:"commented"`
	Discarded       *Descarte        `json:"discarded"`
	Restored        *Restauracion    `json:"restored"`
	AssigneeChanged *CambioAsignado  `json:"assigneeChanged"`
}

// CambioEstado es issues.v1.StatusChanged.
type CambioEstado struct {
	FromStatusName string `json:"fromStatusName"`
	ToStatusName   string `json:"toStatusName"`
}

// CambioTexto sirve para TitleChanged y PriorityChanged (from/to).
type CambioTexto struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// CambioEtiquetas es issues.v1.LabelsChanged.
type CambioEtiquetas struct {
	AddedLabelNames   []string `json:"addedLabelNames"`
	RemovedLabelNames []string `json:"removedLabelNames"`
}

// Descarte es issues.v1.IssueDiscarded.
type Descarte struct {
	Reason       string `json:"reason"`
	ToStatusName string `json:"toStatusName"`
}

// Restauracion es issues.v1.IssueRestored.
type Restauracion struct {
	ToStatusName string `json:"toStatusName"`
}

// CambioAsignado es issues.v1.AssigneeChanged.
type CambioAsignado struct {
	FromDisplayName string `json:"fromDisplayName"`
	ToDisplayName   string `json:"toDisplayName"`
}

type pagina struct {
	NextCursor string `json:"nextCursor"`
	HasNext    bool   `json:"hasNext"`
}

// FiltrosIssues son los filtros de ListIssues que usa tk.
type FiltrosIssues struct {
	StatusIDs  []string // estados concretos (por nombre, resueltos antes)
	Priorities []string // PRIORITY_*
	Query      string
}

// NuevoIssue es el cuerpo de CreateIssue.
type NuevoIssue struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty"`
}
