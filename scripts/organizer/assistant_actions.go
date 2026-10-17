package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

const (
	maxAssistantPlanActions     = 50
	maxAssistantMoveItems       = 100
	maxAssistantDurationMinutes = 43200
)

type AssistantOperation string

const (
	AssistantCreateItem         AssistantOperation = "create_item"
	AssistantUpdateItem         AssistantOperation = "update_item"
	AssistantCompleteItem       AssistantOperation = "complete_item"
	AssistantReopenItem         AssistantOperation = "reopen_item"
	AssistantArchiveItem        AssistantOperation = "archive_item"
	AssistantSetReminder        AssistantOperation = "set_reminder"
	AssistantCreateProject      AssistantOperation = "create_project"
	AssistantUpdateProject      AssistantOperation = "update_project"
	AssistantMergeProjects      AssistantOperation = "merge_projects"
	AssistantMoveItemsToProject AssistantOperation = "move_items_to_project"
	AssistantListItems          AssistantOperation = "list_items"
)

type AssistantRiskLevel string

const (
	AssistantRiskLow    AssistantRiskLevel = "low"
	AssistantRiskMedium AssistantRiskLevel = "medium"
	AssistantRiskHigh   AssistantRiskLevel = "high"
)

// AssistantActionPayload is implemented only by the supported operation payloads.
type AssistantActionPayload interface{ assistantActionPayload() }

type AssistantItemFields struct {
	Type            *string   `json:"type,omitempty"`
	Title           *string   `json:"title,omitempty"`
	Description     *string   `json:"description,omitempty"`
	StartAt         *string   `json:"startAt,omitempty"`
	EndAt           *string   `json:"endAt,omitempty"`
	DueAt           *string   `json:"dueAt,omitempty"`
	ReminderAt      *string   `json:"reminderAt,omitempty"`
	Timezone        *string   `json:"timezone,omitempty"`
	AllDay          *bool     `json:"allDay,omitempty"`
	RecurrenceRule  *string   `json:"recurrenceRule,omitempty"`
	Priority        *int      `json:"priority,omitempty"`
	DurationMinutes *int      `json:"durationMinutes,omitempty"`
	AvailableFrom   *string   `json:"availableFrom,omitempty"`
	AvailableUntil  *string   `json:"availableUntil,omitempty"`
	ProjectID       *string   `json:"projectId,omitempty"`
	Tags            *[]string `json:"tags,omitempty"`
	Location        *string   `json:"location,omitempty"`
	People          *[]string `json:"people,omitempty"`
}

type AssistantCreateItemPayload struct {
	Item AssistantItemFields `json:"item"`
}

type AssistantUpdateItemPayload struct {
	ItemID  string              `json:"itemId"`
	Changes AssistantItemFields `json:"changes"`
}

type AssistantItemTargetPayload struct {
	ItemID string `json:"itemId"`
}

type AssistantSetReminderPayload struct {
	ItemID     string `json:"itemId"`
	ReminderAt string `json:"reminderAt"`
	Timezone   string `json:"timezone"`
}

type AssistantProjectFields struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
	Color       *string `json:"color,omitempty"`
	Goal        *string `json:"goal,omitempty"`
	Deadline    *string `json:"deadline,omitempty"`
	ReviewAt    *string `json:"reviewAt,omitempty"`
}

type AssistantCreateProjectPayload struct {
	Project AssistantProjectFields `json:"project"`
}

type AssistantUpdateProjectPayload struct {
	ProjectID string                 `json:"projectId"`
	Changes   AssistantProjectFields `json:"changes"`
}

type AssistantMergeProjectsPayload struct {
	SourceProjectID       string `json:"sourceProjectId"`
	TargetProjectID       string `json:"targetProjectId"`
	SourceExpectedVersion *int   `json:"sourceExpectedVersion,omitempty"`
}

type AssistantMoveItemsPayload struct {
	ItemIDs          []string       `json:"itemIds"`
	TargetProjectID  string         `json:"targetProjectId"`
	ExpectedVersions map[string]int `json:"expectedVersions,omitempty"`
}

type AssistantListItemsPayload struct {
	Status    string `json:"status,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}

func (AssistantCreateItemPayload) assistantActionPayload()    {}
func (AssistantUpdateItemPayload) assistantActionPayload()    {}
func (AssistantItemTargetPayload) assistantActionPayload()    {}
func (AssistantSetReminderPayload) assistantActionPayload()   {}
func (AssistantCreateProjectPayload) assistantActionPayload() {}
func (AssistantUpdateProjectPayload) assistantActionPayload() {}
func (AssistantMergeProjectsPayload) assistantActionPayload() {}
func (AssistantMoveItemsPayload) assistantActionPayload()     {}
func (AssistantListItemsPayload) assistantActionPayload()     {}

// AssistantTargetReader resolves IDs only within the requested workspace.
// Implementations must return the current version for stale-plan detection.
type AssistantTargetReader interface {
	AssistantItemVersion(workspaceID, itemID string) (int, error)
	AssistantProjectVersion(workspaceID, projectID string) (int, error)
}

func (a *AssistantAction) UnmarshalJSON(data []byte) error {
	var envelope struct {
		ActionID               string             `json:"actionId"`
		Operation              AssistantOperation `json:"operation"`
		ExpectedItemVersion    *int               `json:"expectedItemVersion,omitempty"`
		ExpectedItemVersions   map[string]int     `json:"expectedItemVersions,omitempty"`
		ExpectedProjectVersion *int               `json:"expectedProjectVersion,omitempty"`
		Payload                json.RawMessage    `json:"payload"`
	}
	if err := decodeStrict(data, &envelope); err != nil {
		return err
	}
	var payload AssistantActionPayload
	switch envelope.Operation {
	case AssistantCreateItem:
		payload = &AssistantCreateItemPayload{}
	case AssistantUpdateItem:
		payload = &AssistantUpdateItemPayload{}
	case AssistantCompleteItem, AssistantReopenItem, AssistantArchiveItem:
		payload = &AssistantItemTargetPayload{}
	case AssistantSetReminder:
		payload = &AssistantSetReminderPayload{}
	case AssistantCreateProject:
		payload = &AssistantCreateProjectPayload{}
	case AssistantUpdateProject:
		payload = &AssistantUpdateProjectPayload{}
	case AssistantMergeProjects:
		payload = &AssistantMergeProjectsPayload{}
	case AssistantMoveItemsToProject:
		payload = &AssistantMoveItemsPayload{}
	case AssistantListItems:
		payload = &AssistantListItemsPayload{}
	default:
		return fmt.Errorf("unknown assistant operation %q", envelope.Operation)
	}
	if len(envelope.Payload) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Payload), []byte("null")) {
		return errors.New("assistant action payload is required")
	}
	if err := decodeStrict(envelope.Payload, payload); err != nil {
		return fmt.Errorf("invalid %s payload: %w", envelope.Operation, err)
	}
	*a = AssistantAction{ActionID: envelope.ActionID, Operation: envelope.Operation,
		ExpectedItemVersion: envelope.ExpectedItemVersion, ExpectedProjectVersion: envelope.ExpectedProjectVersion,
		ExpectedItemVersions: envelope.ExpectedItemVersions,
		Payload:              payload}
	return nil
}

func DecodeAssistantPlan(data []byte) (AssistantPlan, error) {
	var plan AssistantPlan
	if err := decodeStrict(data, &plan); err != nil {
		return plan, fmt.Errorf("invalid assistant plan JSON: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return AssistantPlan{}, err
	}
	return plan, nil
}

func (p AssistantPlan) Validate() error {
	kind := p.Kind
	if kind == "" {
		kind = "plan"
	}
	switch kind {
	case "answer", "query", "plan", "clarification":
	default:
		return fmt.Errorf("invalid assistant plan kind %q", p.Kind)
	}
	if strings.TrimSpace(p.IntentSummary) == "" {
		return errors.New("assistant plan intentSummary is required")
	}
	switch p.RiskLevel {
	case AssistantRiskLow, AssistantRiskMedium, AssistantRiskHigh:
	default:
		return fmt.Errorf("invalid assistant plan riskLevel %q", p.RiskLevel)
	}
	if kind == "answer" && strings.TrimSpace(p.Reply) == "" {
		return errors.New("assistant answer reply is required")
	}
	if kind == "answer" || kind == "clarification" {
		if len(p.Actions) != 0 {
			return fmt.Errorf("assistant %s must not contain actions", kind)
		}
		if kind == "clarification" && len(p.AmbiguityQuestions) == 0 {
			return errors.New("assistant clarification requires ambiguityQuestions")
		}
		return nil
	}
	if len(p.Actions) == 0 || len(p.Actions) > maxAssistantPlanActions {
		return fmt.Errorf("assistant plan must contain 1 to %d actions", maxAssistantPlanActions)
	}
	for i, question := range p.AmbiguityQuestions {
		if strings.TrimSpace(question) == "" {
			return fmt.Errorf("ambiguityQuestions[%d] must not be empty", i)
		}
	}
	seen := make(map[string]struct{}, len(p.Actions))
	for i := range p.Actions {
		a := &p.Actions[i]
		if err := a.Validate(); err != nil {
			return fmt.Errorf("actions[%d]: %w", i, err)
		}
		if _, ok := seen[a.ActionID]; ok {
			return fmt.Errorf("duplicate assistant actionId %q", a.ActionID)
		}
		seen[a.ActionID] = struct{}{}
	}
	return nil
}

func (a AssistantAction) Validate() error {
	if strings.TrimSpace(a.ActionID) == "" || len(a.ActionID) > 128 {
		return errors.New("actionId is required and must be at most 128 characters")
	}
	if a.ExpectedItemVersion != nil && *a.ExpectedItemVersion < 1 {
		return errors.New("expectedItemVersion must be positive")
	}
	if a.ExpectedProjectVersion != nil && *a.ExpectedProjectVersion < 1 {
		return errors.New("expectedProjectVersion must be positive")
	}
	for id, version := range a.ExpectedItemVersions {
		if err := requiredID("itemId", id); err != nil {
			return err
		}
		if version < 1 {
			return fmt.Errorf("expectedItemVersions[%q] must be positive", id)
		}
	}
	if a.Payload == nil {
		return errors.New("payload is required")
	}
	if !knownAssistantOperation(a.Operation) {
		return fmt.Errorf("unknown assistant operation %q", a.Operation)
	}
	if !payloadMatchesOperation(a.Operation, a.Payload) {
		return fmt.Errorf("operation %q has a mismatched payload type %T", a.Operation, a.Payload)
	}
	switch payload := a.Payload.(type) {
	case *AssistantCreateItemPayload:
		if a.ExpectedItemVersion != nil || a.ExpectedProjectVersion != nil {
			return errors.New("create_item cannot specify expected versions")
		}
		if err := validateItemFields(payload.Item, true); err != nil {
			return err
		}
	case *AssistantUpdateItemPayload:
		if err := requiredID("itemId", payload.ItemID); err != nil {
			return err
		}
		if a.ExpectedProjectVersion != nil {
			return errors.New("update_item cannot specify expectedProjectVersion")
		}
		if !hasItemFields(payload.Changes) {
			return errors.New("update_item changes must not be empty")
		}
		if err := validateItemFields(payload.Changes, false); err != nil {
			return err
		}
		if payload.Changes.ProjectID != nil && *payload.Changes.ProjectID != "" {
			if err := requiredID("projectId", *payload.Changes.ProjectID); err != nil {
				return err
			}
		}
	case *AssistantItemTargetPayload:
		if err := requiredID("itemId", payload.ItemID); err != nil {
			return err
		}
		if a.ExpectedProjectVersion != nil {
			return errors.New("item action cannot specify expectedProjectVersion")
		}
	case *AssistantSetReminderPayload:
		if err := requiredID("itemId", payload.ItemID); err != nil {
			return err
		}
		if a.ExpectedProjectVersion != nil {
			return errors.New("set_reminder cannot specify expectedProjectVersion")
		}
		if err := validateTimezone(payload.Timezone); err != nil {
			return err
		}
		if err := validateOptionalTime(payload.ReminderAt, false); err != nil {
			return fmt.Errorf("reminderAt: %w", err)
		}
	case *AssistantCreateProjectPayload:
		if a.ExpectedItemVersion != nil || a.ExpectedProjectVersion != nil {
			return errors.New("create_project cannot specify expected versions")
		}
		if err := validateProjectFields(payload.Project, true); err != nil {
			return err
		}
	case *AssistantUpdateProjectPayload:
		if err := requiredID("projectId", payload.ProjectID); err != nil {
			return err
		}
		if a.ExpectedItemVersion != nil {
			return errors.New("update_project cannot specify expectedItemVersion")
		}
		if !hasProjectFields(payload.Changes) {
			return errors.New("update_project changes must not be empty")
		}
		if err := validateProjectFields(payload.Changes, false); err != nil {
			return err
		}
	case *AssistantMergeProjectsPayload:
		if err := requiredID("sourceProjectId", payload.SourceProjectID); err != nil {
			return err
		}
		if err := requiredID("targetProjectId", payload.TargetProjectID); err != nil {
			return err
		}
		if payload.SourceProjectID == payload.TargetProjectID {
			return errors.New("source and target projects must differ")
		}
		if payload.SourceExpectedVersion != nil && *payload.SourceExpectedVersion < 1 {
			return errors.New("sourceExpectedVersion must be positive")
		}
		if a.ExpectedItemVersion != nil {
			return errors.New("merge_projects cannot specify expectedItemVersion")
		}
	case *AssistantMoveItemsPayload:
		if err := requiredID("targetProjectId", payload.TargetProjectID); err != nil {
			return err
		}
		if len(payload.ItemIDs) == 0 || len(payload.ItemIDs) > maxAssistantMoveItems {
			return fmt.Errorf("itemIds must contain 1 to %d IDs", maxAssistantMoveItems)
		}
		seen := make(map[string]struct{}, len(payload.ItemIDs))
		for _, id := range payload.ItemIDs {
			if err := requiredID("itemId", id); err != nil {
				return err
			}
			if _, exists := seen[id]; exists {
				return fmt.Errorf("duplicate itemId %q", id)
			}
			seen[id] = struct{}{}
		}
		if len(payload.ExpectedVersions) != 0 && len(payload.ExpectedVersions) != len(payload.ItemIDs) {
			return errors.New("expectedVersions must include exactly one version per itemId")
		}
		for id, version := range payload.ExpectedVersions {
			if _, exists := seen[id]; !exists {
				return fmt.Errorf("expectedVersions contains unknown itemId %q", id)
			}
			if version < 1 {
				return fmt.Errorf("expectedVersions[%q] must be positive", id)
			}
		}
	case *AssistantListItemsPayload:
		if a.ExpectedItemVersion != nil || a.ExpectedProjectVersion != nil || len(a.ExpectedItemVersions) != 0 {
			return errors.New("list_items cannot specify expected versions")
		}
		if payload.Status != "" && payload.Status != "inbox" && payload.Status != "todo" && payload.Status != "doing" && payload.Status != "done" && payload.Status != "cancelled" && payload.Status != "archived" {
			return fmt.Errorf("invalid item status %q", payload.Status)
		}
		if payload.ProjectID != "" {
			if err := requiredID("projectId", payload.ProjectID); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported payload type %T", a.Payload)
	}
	return nil
}

func knownAssistantOperation(operation AssistantOperation) bool {
	switch operation {
	case AssistantCreateItem, AssistantUpdateItem, AssistantCompleteItem, AssistantReopenItem, AssistantArchiveItem,
		AssistantSetReminder, AssistantCreateProject, AssistantUpdateProject, AssistantMergeProjects, AssistantMoveItemsToProject:
		return true
	case AssistantListItems:
		return true
	default:
		return false
	}
}

func payloadMatchesOperation(operation AssistantOperation, payload AssistantActionPayload) bool {
	switch operation {
	case AssistantCreateItem:
		_, ok := payload.(*AssistantCreateItemPayload)
		return ok
	case AssistantUpdateItem:
		_, ok := payload.(*AssistantUpdateItemPayload)
		return ok
	case AssistantCompleteItem, AssistantReopenItem, AssistantArchiveItem:
		_, ok := payload.(*AssistantItemTargetPayload)
		return ok
	case AssistantSetReminder:
		_, ok := payload.(*AssistantSetReminderPayload)
		return ok
	case AssistantCreateProject:
		_, ok := payload.(*AssistantCreateProjectPayload)
		return ok
	case AssistantUpdateProject:
		_, ok := payload.(*AssistantUpdateProjectPayload)
		return ok
	case AssistantMergeProjects:
		_, ok := payload.(*AssistantMergeProjectsPayload)
		return ok
	case AssistantMoveItemsToProject:
		_, ok := payload.(*AssistantMoveItemsPayload)
		return ok
	case AssistantListItems:
		_, ok := payload.(*AssistantListItemsPayload)
		return ok
	default:
		return false
	}
}

// ValidateTargets verifies that each referenced object belongs to workspaceID and
// that any supplied expected version still matches. It performs no writes.
func (a AssistantAction) ValidateTargets(workspaceID string, reader AssistantTargetReader) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if payload, ok := a.Payload.(*AssistantListItemsPayload); ok {
		if payload.ProjectID == "" {
			return nil
		}
		if strings.TrimSpace(workspaceID) == "" || reader == nil {
			return errors.New("workspace and target reader are required")
		}
		if _, err := reader.AssistantProjectVersion(workspaceID, payload.ProjectID); err != nil {
			return fmt.Errorf("project %q is missing from workspace: %w", payload.ProjectID, err)
		}
		return nil
	}
	if strings.TrimSpace(workspaceID) == "" || reader == nil {
		return errors.New("workspace and target reader are required")
	}
	checkItem := func(id string, expected *int) error {
		version, err := reader.AssistantItemVersion(workspaceID, id)
		if err != nil {
			return fmt.Errorf("item %q is missing from workspace: %w", id, err)
		}
		if expected != nil && version != *expected {
			return fmt.Errorf("item %q has stale version %d (expected %d)", id, version, *expected)
		}
		return nil
	}
	checkProject := func(id string, expected *int) error {
		version, err := reader.AssistantProjectVersion(workspaceID, id)
		if err != nil {
			return fmt.Errorf("project %q is missing from workspace: %w", id, err)
		}
		if expected != nil && version != *expected {
			return fmt.Errorf("project %q has stale version %d (expected %d)", id, version, *expected)
		}
		return nil
	}
	switch payload := a.Payload.(type) {
	case *AssistantCreateItemPayload:
		if payload.Item.ProjectID != nil && *payload.Item.ProjectID != "" {
			return checkProject(*payload.Item.ProjectID, nil)
		}
	case *AssistantUpdateItemPayload:
		if err := checkItem(payload.ItemID, a.ExpectedItemVersion); err != nil {
			return err
		}
		if payload.Changes.ProjectID != nil && *payload.Changes.ProjectID != "" {
			return checkProject(*payload.Changes.ProjectID, nil)
		}
	case *AssistantItemTargetPayload:
		return checkItem(payload.ItemID, a.ExpectedItemVersion)
	case *AssistantSetReminderPayload:
		return checkItem(payload.ItemID, a.ExpectedItemVersion)
	case *AssistantUpdateProjectPayload:
		return checkProject(payload.ProjectID, a.ExpectedProjectVersion)
	case *AssistantMergeProjectsPayload:
		if err := checkProject(payload.SourceProjectID, payload.SourceExpectedVersion); err != nil {
			return err
		}
		for id, expected := range a.ExpectedItemVersions {
			if err := checkItem(id, &expected); err != nil {
				return err
			}
		}
		return checkProject(payload.TargetProjectID, a.ExpectedProjectVersion)
	case *AssistantMoveItemsPayload:
		for _, id := range payload.ItemIDs {
			expected, ok := payload.ExpectedVersions[id]
			if !ok && a.ExpectedItemVersion != nil {
				expected = *a.ExpectedItemVersion
				ok = true
			}
			var expectedPtr *int
			if ok {
				expectedPtr = &expected
			}
			if err := checkItem(id, expectedPtr); err != nil {
				return err
			}
		}
		return checkProject(payload.TargetProjectID, a.ExpectedProjectVersion)
	default:
		return nil
	}
	return nil
}

func validateItemFields(f AssistantItemFields, create bool) error {
	if create {
		if f.Type == nil || *f.Type == "" {
			return errors.New("item.type is required")
		}
		if f.Title == nil || strings.TrimSpace(*f.Title) == "" {
			return errors.New("item.title is required")
		}
	}
	if f.Type != nil && *f.Type != "task" && *f.Type != "event" && *f.Type != "reminder" && *f.Type != "note" {
		return fmt.Errorf("invalid item type %q", *f.Type)
	}
	if f.Title != nil && strings.TrimSpace(*f.Title) == "" {
		return errors.New("item title must not be empty")
	}
	if f.Priority != nil && (*f.Priority < 0 || *f.Priority > 3) {
		return errors.New("priority must be between 0 and 3")
	}
	if f.DurationMinutes != nil && (*f.DurationMinutes < 0 || *f.DurationMinutes > maxAssistantDurationMinutes) {
		return fmt.Errorf("durationMinutes must be between 0 and %d", maxAssistantDurationMinutes)
	}
	if f.Timezone != nil {
		if err := validateTimezone(*f.Timezone); err != nil {
			return err
		}
	}
	for name, value := range map[string]*string{"startAt": f.StartAt, "endAt": f.EndAt, "dueAt": f.DueAt, "reminderAt": f.ReminderAt, "availableFrom": f.AvailableFrom, "availableUntil": f.AvailableUntil} {
		if value != nil {
			if err := validateOptionalTime(*value, true); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

func hasItemFields(f AssistantItemFields) bool {
	return f.Type != nil || f.Title != nil || f.Description != nil || f.StartAt != nil || f.EndAt != nil ||
		f.DueAt != nil || f.ReminderAt != nil || f.Timezone != nil || f.AllDay != nil || f.RecurrenceRule != nil ||
		f.Priority != nil || f.DurationMinutes != nil || f.AvailableFrom != nil || f.AvailableUntil != nil ||
		f.ProjectID != nil || f.Tags != nil || f.Location != nil || f.People != nil
}

func validateProjectFields(f AssistantProjectFields, create bool) error {
	if create && (f.Name == nil || strings.TrimSpace(*f.Name) == "") {
		return errors.New("project.name is required")
	}
	if f.Name != nil && strings.TrimSpace(*f.Name) == "" {
		return errors.New("project name must not be empty")
	}
	if f.Priority != nil && (*f.Priority < 0 || *f.Priority > 3) {
		return errors.New("priority must be between 0 and 3")
	}
	if f.Status != nil && *f.Status != "active" && *f.Status != "waiting" && *f.Status != "blocked" && *f.Status != "completed" && *f.Status != "archived" {
		return fmt.Errorf("invalid project status %q", *f.Status)
	}
	for name, value := range map[string]*string{"deadline": f.Deadline, "reviewAt": f.ReviewAt} {
		if value != nil {
			if err := validateOptionalTime(*value, true); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

func hasProjectFields(f AssistantProjectFields) bool {
	return f.Name != nil || f.Description != nil || f.Status != nil || f.Priority != nil || f.Color != nil ||
		f.Goal != nil || f.Deadline != nil || f.ReviewAt != nil
}

func validateTimezone(zone string) error {
	if strings.TrimSpace(zone) == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("invalid timezone %q", zone)
	}
	return nil
}

func validateOptionalTime(value string, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("must be RFC3339: %w", err)
	}
	return nil
}

func requiredID(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("%s must not contain whitespace", name)
	}
	return nil
}

func decodeStrict(data []byte, dst any) error {
	if err := validateJSONFieldNames(data, reflect.TypeOf(dst)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateJSONFieldNames(data []byte, target reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, target reflect.Type) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	for target != nil && (target.Kind() == reflect.Pointer || target.Kind() == reflect.Interface) {
		if target.Kind() == reflect.Interface {
			target = nil
			break
		}
		target = target.Elem()
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			fields := make(map[string]reflect.Type)
			foldedFields := make(map[string]string)
			if target != nil && target.Kind() == reflect.Struct {
				for i := 0; i < target.NumField(); i++ {
					field := target.Field(i)
					if !field.IsExported() {
						continue
					}
					name := strings.Split(field.Tag.Get("json"), ",")[0]
					if name == "-" {
						continue
					}
					if name == "" {
						name = field.Name
					}
					fields[name] = field.Type
					foldedFields[strings.ToLower(name)] = name
				}
			}
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key must be a string")
				}
				child := reflect.Type(nil)
				identity := key
				if target != nil && target.Kind() == reflect.Struct {
					var exists bool
					child, exists = fields[key]
					if !exists {
						if canonical, known := foldedFields[strings.ToLower(key)]; known {
							return fmt.Errorf("field %q must use canonical spelling %q", key, canonical)
						}
						return fmt.Errorf("json: unknown field %q", key)
					}
					identity = strings.ToLower(key)
				} else if target != nil && target.Kind() == reflect.Map {
					child = target.Elem()
				}
				if _, exists := seen[identity]; exists {
					return fmt.Errorf("duplicate JSON field %q", key)
				}
				seen[identity] = struct{}{}
				if err := scanJSONValue(decoder, child); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil {
				return err
			}
			if closing != json.Delim('}') {
				return errors.New("malformed JSON object")
			}
		case '[':
			var element reflect.Type
			if target != nil && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
				element = target.Elem()
			}
			for decoder.More() {
				if err := scanJSONValue(decoder, element); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil {
				return err
			}
			if closing != json.Delim(']') {
				return errors.New("malformed JSON array")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
	}
	return nil
}
