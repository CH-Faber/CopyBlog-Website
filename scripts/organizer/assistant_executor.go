package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	assistantConfirmationTTL   = 10 * time.Minute
	assistantExecutionLeaseTTL = 2 * time.Minute
	// Plans affecting more than five actions require a deliberate confirmation.
	assistantBulkConfirmationThreshold = 5
)

// AssistantExecutor persists previews and applies only the closed action
// vocabulary decoded by assistant_actions.go.
type AssistantExecutor struct{ store *Store }

var beforeAssistantActionHook func()
var afterAssistantActionMutationHook func()

func NewAssistantExecutor(store *Store) *AssistantExecutor { return &AssistantExecutor{store: store} }

func assistantPlanNeedsConfirmation(plan AssistantPlan) bool {
	if plan.Kind == "answer" || plan.Kind == "clarification" {
		return false
	}
	readOnly := len(plan.Actions) > 0
	for _, action := range plan.Actions {
		if action.Operation != AssistantListItems {
			readOnly = false
			break
		}
	}
	if readOnly {
		return len(plan.AmbiguityQuestions) > 0
	}
	// Every mutation is previewed first. The assistant may understand a low-risk
	// request correctly, but the user remains the final authority over changes
	// to their personal schedule. The existing risk and bulk checks still shape
	// the preview metadata and confirmation expiry.
	return len(plan.Actions) > 0
}

func normalizedAssistantPlan(plan AssistantPlan) (AssistantPlan, error) {
	if err := plan.Validate(); err != nil {
		return AssistantPlan{}, err
	}
	plan.IntentSummary = strings.TrimSpace(plan.IntentSummary)
	for i := range plan.AmbiguityQuestions {
		plan.AmbiguityQuestions[i] = strings.TrimSpace(plan.AmbiguityQuestions[i])
	}
	for i := range plan.Actions {
		plan.Actions[i].ActionID = strings.TrimSpace(plan.Actions[i].ActionID)
	}
	return plan, nil
}

func assistantPlanHash(plan AssistantPlan) (string, error) {
	normalized, err := normalizedAssistantPlan(plan)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) ensureAssistantWorkspace(workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return errors.New("workspace is required")
	}
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id=?", workspaceID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("workspace %q not found", workspaceID)
	}
	return nil
}

func (s *Store) saveAssistantPlan(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	if err := s.ensureAssistantWorkspace(req.WorkspaceID); err != nil {
		return AssistantPlanRecord{}, err
	}
	if req.WorkspaceID != s.defaultWorkspaceID() {
		return AssistantPlanRecord{}, errors.New("assistant plan workspace is not active")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" || len(req.IdempotencyKey) > 256 {
		return AssistantPlanRecord{}, errors.New("idempotency key is required and must be at most 256 characters")
	}
	plan, err := normalizedAssistantPlan(req.Plan)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	hash, err := assistantPlanHash(plan)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	// Resolve ownership and the versions captured in the preview before any
	// plan is persisted. Execution repeats this check to catch later edits.
	if req.WorkspaceID == s.defaultWorkspaceID() {
		for i := range plan.Actions {
			if err := s.captureAssistantActionVersions(req.WorkspaceID, &plan.Actions[i]); err != nil {
				return AssistantPlanRecord{}, fmt.Errorf("actions[%d]: %w", i, err)
			}
			if plan.Actions[i].Operation == AssistantMergeProjects {
				if payload, ok := plan.Actions[i].Payload.(*AssistantMergeProjectsPayload); ok && payload.SourceExpectedVersion == nil {
					version, versionErr := s.AssistantProjectVersion(req.WorkspaceID, payload.SourceProjectID)
					if versionErr != nil {
						return AssistantPlanRecord{}, versionErr
					}
					payload.SourceExpectedVersion = &version
				}
				if len(plan.Actions[i].ExpectedItemVersions) == 0 {
					if payload, ok := plan.Actions[i].Payload.(*AssistantMergeProjectsPayload); ok {
						items, listErr := s.listItems("", 10000)
						if listErr != nil {
							return AssistantPlanRecord{}, listErr
						}
						versions := map[string]int{}
						for _, item := range items {
							if item.ProjectID == payload.SourceProjectID {
								versions[item.ID] = item.Version
							}
						}
						plan.Actions[i].ExpectedItemVersions = versions
					}
				}
			}
			if err := plan.Actions[i].ValidateTargets(req.WorkspaceID, s); err != nil {
				return AssistantPlanRecord{}, fmt.Errorf("actions[%d]: %w", i, err)
			}
		}
	}
	if req.Actor == "" {
		req.Actor = "assistant"
	}
	if req.Actor != "assistant" {
		return AssistantPlanRecord{}, errors.New("assistant plan actor must be assistant")
	}
	if len(req.Actor) > 128 {
		return AssistantPlanRecord{}, errors.New("actor is too long")
	}
	if req.ConversationID != "" {
		if _, err := s.getAssistantConversationInWorkspace(req.WorkspaceID, req.ConversationID); err != nil {
			return AssistantPlanRecord{}, err
		}
	}
	requires := assistantPlanNeedsConfirmation(plan)
	now := nowString()
	expires := ""
	state := AssistantPlanProposed
	if requires {
		state = AssistantPlanAwaitingConfirmation
		expires = time.Now().UTC().Add(assistantConfirmationTTL).Format(time.RFC3339)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	defer tx.Rollback()
	var existingID, existingConversationID, existingHash, existingSourceHash string
	err = tx.QueryRow("SELECT id, conversation_id, request_hash, source_request_hash FROM assistant_plans WHERE workspace_id=? AND idempotency_key=?", req.WorkspaceID, req.IdempotencyKey).Scan(&existingID, &existingConversationID, &existingHash, &existingSourceHash)
	if err == nil {
		if existingHash != hash || existingConversationID != req.ConversationID || (req.SourceRequestHash != "" && existingSourceHash != req.SourceRequestHash) {
			return AssistantPlanRecord{}, errors.New("idempotency key already used for a different request")
		}
		if err := tx.Commit(); err != nil {
			return AssistantPlanRecord{}, err
		}
		return s.getAssistantPlan(req.WorkspaceID, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AssistantPlanRecord{}, err
	}
	id := newID("plan")
	if _, err := tx.Exec(`INSERT INTO assistant_plans(id, workspace_id, conversation_id, turn_id, idempotency_key, request_hash, source_request_hash, state, plan_json, requires_confirmation, confirmation_expires_at, actor, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, req.WorkspaceID, req.ConversationID, req.TurnID, req.IdempotencyKey, hash, req.SourceRequestHash, state, string(planJSON), requires, expires, req.Actor, now, now); err != nil {
		var concurrentID, concurrentConversationID, concurrentHash, concurrentSourceHash string
		if queryErr := tx.QueryRow("SELECT id, conversation_id, request_hash, source_request_hash FROM assistant_plans WHERE workspace_id=? AND idempotency_key=?", req.WorkspaceID, req.IdempotencyKey).Scan(&concurrentID, &concurrentConversationID, &concurrentHash, &concurrentSourceHash); queryErr == nil {
			if concurrentHash != hash || concurrentConversationID != req.ConversationID || (req.SourceRequestHash != "" && concurrentSourceHash != req.SourceRequestHash) {
				return AssistantPlanRecord{}, errors.New("idempotency key already used for a different request")
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return AssistantPlanRecord{}, commitErr
			}
			return s.getAssistantPlan(req.WorkspaceID, concurrentID)
		}
		return AssistantPlanRecord{}, err
	}
	for ordinal, action := range plan.Actions {
		payload, err := json.Marshal(action.Payload)
		if err != nil {
			return AssistantPlanRecord{}, err
		}
		if _, err := tx.Exec(`INSERT INTO assistant_plan_actions(plan_id, workspace_id, action_id, ordinal, operation, payload_json, actor, conversation_id, turn_id)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, req.WorkspaceID, action.ActionID, ordinal, action.Operation, string(payload), req.Actor, req.ConversationID, req.TurnID); err != nil {
			return AssistantPlanRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AssistantPlanRecord{}, err
	}
	return s.getAssistantPlan(req.WorkspaceID, id)
}

func (s *Store) captureAssistantActionVersions(workspaceID string, action *AssistantAction) error {
	itemVersion := func(id string, target **int) error {
		if *target != nil {
			return nil
		}
		version, err := s.AssistantItemVersion(workspaceID, id)
		if err != nil {
			return err
		}
		*target = &version
		return nil
	}
	projectVersion := func(id string, target **int) error {
		if *target != nil {
			return nil
		}
		version, err := s.AssistantProjectVersion(workspaceID, id)
		if err != nil {
			return err
		}
		*target = &version
		return nil
	}
	switch payload := action.Payload.(type) {
	case *AssistantUpdateItemPayload:
		return itemVersion(payload.ItemID, &action.ExpectedItemVersion)
	case *AssistantItemTargetPayload:
		return itemVersion(payload.ItemID, &action.ExpectedItemVersion)
	case *AssistantSetReminderPayload:
		return itemVersion(payload.ItemID, &action.ExpectedItemVersion)
	case *AssistantUpdateProjectPayload:
		return projectVersion(payload.ProjectID, &action.ExpectedProjectVersion)
	case *AssistantMergeProjectsPayload:
		if err := projectVersion(payload.SourceProjectID, &payload.SourceExpectedVersion); err != nil {
			return err
		}
		return projectVersion(payload.TargetProjectID, &action.ExpectedProjectVersion)
	case *AssistantMoveItemsPayload:
		if payload.ExpectedVersions == nil {
			payload.ExpectedVersions = map[string]int{}
		}
		for _, id := range payload.ItemIDs {
			if _, ok := payload.ExpectedVersions[id]; !ok {
				version, err := s.AssistantItemVersion(workspaceID, id)
				if err != nil {
					return err
				}
				payload.ExpectedVersions[id] = version
			}
		}
		return projectVersion(payload.TargetProjectID, &action.ExpectedProjectVersion)
	default:
		return nil
	}
}

func (s *Store) getAssistantConversationInWorkspace(workspaceID, id string) (AssistantConversation, error) {
	var c AssistantConversation
	err := s.db.QueryRow(`SELECT id, workspace_id, status, provider, model, created_at, updated_at FROM assistant_conversations WHERE workspace_id=? AND id=?`, workspaceID, id).Scan(&c.ID, &c.WorkspaceID, &c.Status, &c.Provider, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) getAssistantPlan(workspaceID, id string) (AssistantPlanRecord, error) {
	if err := s.ensureAssistantWorkspace(workspaceID); err != nil {
		return AssistantPlanRecord{}, err
	}
	var r AssistantPlanRecord
	var planText string
	var requires int
	err := s.db.QueryRow(`SELECT id, workspace_id, conversation_id, turn_id, idempotency_key, request_hash, source_request_hash, state, plan_json, requires_confirmation, confirmation_expires_at, actor, error, created_at, updated_at, started_at, completed_at, lease_until FROM assistant_plans WHERE workspace_id=? AND id=?`, workspaceID, id).Scan(&r.ID, &r.WorkspaceID, &r.ConversationID, &r.TurnID, &r.IdempotencyKey, &r.RequestHash, &r.SourceRequestHash, &r.State, &planText, &requires, &r.ConfirmationExpiresAt, &r.Actor, &r.Error, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.CompletedAt, &r.LeaseUntil)
	if err != nil {
		return r, err
	}
	if err := decodePersistedAssistantPlan([]byte(planText), &r.Plan); err != nil {
		return r, err
	}
	r.RequiresConfirmation = requires != 0
	rows, err := s.db.Query(`SELECT action_id, operation, state, error, before_json, after_json, result_json, event_ids_json, actor, conversation_id, turn_id, started_at, completed_at FROM assistant_plan_actions WHERE plan_id=? AND workspace_id=? ORDER BY ordinal`, r.ID, workspaceID)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var a AssistantActionResult
		var before, after, result, eventIDs string
		if err := rows.Scan(&a.ActionID, &a.Operation, &a.State, &a.Error, &before, &after, &result, &eventIDs, &a.Actor, &a.ConversationID, &a.TurnID, &a.StartedAt, &a.CompletedAt); err != nil {
			return r, err
		}
		if before != "" {
			a.BeforeSnapshot = json.RawMessage(before)
		}
		if after != "" {
			a.AfterSnapshot = json.RawMessage(after)
		}
		if result != "" {
			a.Result = json.RawMessage(result)
		}
		_ = json.Unmarshal([]byte(eventIDs), &a.EventIDs)
		r.Actions = append(r.Actions, a)
	}
	return r, rows.Err()
}

func decodePersistedAssistantPlan(data []byte, dst *AssistantPlan) error {
	var envelope struct {
		Kind                 string             `json:"kind"`
		Reply                string             `json:"reply"`
		IntentSummary        string             `json:"intentSummary"`
		AmbiguityQuestions   []string           `json:"ambiguityQuestions"`
		RiskLevel            AssistantRiskLevel `json:"riskLevel"`
		RequiresConfirmation bool               `json:"requiresConfirmation"`
		Actions              []json.RawMessage  `json:"actions"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	decoded := AssistantPlan{Kind: envelope.Kind, Reply: envelope.Reply, IntentSummary: envelope.IntentSummary, AmbiguityQuestions: envelope.AmbiguityQuestions, RiskLevel: envelope.RiskLevel, RequiresConfirmation: envelope.RequiresConfirmation}
	for _, raw := range envelope.Actions {
		var a AssistantAction
		if err := decodeStrict(raw, &a); err != nil {
			return err
		}
		decoded.Actions = append(decoded.Actions, a)
	}
	*dst = decoded
	return nil
}

func (e *AssistantExecutor) Preview(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	return e.store.saveAssistantPlan(req)
}

func (e *AssistantExecutor) Execute(req AssistantExecuteRequest) (AssistantPlanRecord, error) {
	if e == nil || e.store == nil {
		return AssistantPlanRecord{}, errors.New("assistant executor is required")
	}
	if err := e.store.ensureAssistantWorkspace(req.WorkspaceID); err != nil {
		return AssistantPlanRecord{}, err
	}
	var record AssistantPlanRecord
	var err error
	if req.PlanID != "" {
		record, err = e.store.getAssistantPlan(req.WorkspaceID, req.PlanID)
	} else if req.IdempotencyKey != "" {
		record, err = e.store.getAssistantPlanByIdempotency(req.WorkspaceID, req.IdempotencyKey)
	} else {
		err = errors.New("planId or idempotencyKey is required")
	}
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	if req.Actor == "" {
		req.Actor = record.Actor
	}
	if req.Actor == "" {
		req.Actor = "assistant"
	}
	if req.Actor != "assistant" {
		return AssistantPlanRecord{}, errors.New("assistant plan actor must be assistant")
	}
	if record.State == AssistantPlanCancelled || record.State == AssistantPlanCompleted || record.State == AssistantPlanPartiallyFailed || record.State == AssistantPlanStale {
		return record, nil
	}
	if record.RequiresConfirmation && record.ConfirmationExpiresAt != "" {
		expiry, parseErr := time.Parse(time.RFC3339, record.ConfirmationExpiresAt)
		if parseErr != nil || time.Now().UTC().After(expiry) {
			return e.store.expireAssistantPlan(record)
		}
	}
	if record.State == AssistantPlanExecuting && record.LeaseUntil != "" {
		lease, parseErr := time.Parse(time.RFC3339Nano, record.LeaseUntil)
		if parseErr == nil && time.Now().UTC().Before(lease) {
			return record, nil
		}
	}
	if record.RequiresConfirmation {
		if record.ConfirmationExpiresAt != "" {
			expiry, parseErr := time.Parse(time.RFC3339, record.ConfirmationExpiresAt)
			if parseErr != nil || time.Now().UTC().After(expiry) {
				return e.store.expireAssistantPlan(record)
			}
		}
		if !req.Confirmed {
			return record, nil
		}
	}
	leaseToken := ""
	if record.State == AssistantPlanAwaitingConfirmation || record.State == AssistantPlanProposed || record.State == AssistantPlanExecuting {
		started := nowString()
		leaseUntil := time.Now().UTC().Add(assistantExecutionLeaseTTL).Format(time.RFC3339Nano)
		leaseToken = newID("lease")
		res, err := e.store.db.Exec("UPDATE assistant_plans SET state=?, started_at=CASE WHEN started_at='' THEN ? ELSE started_at END, updated_at=?, actor=?, lease_until=?, lease_token=? WHERE id=? AND workspace_id=? AND (state IN (?, ?) OR (state=? AND (lease_until='' OR lease_until<=?)))", AssistantPlanExecuting, started, started, req.Actor, leaseUntil, leaseToken, record.ID, req.WorkspaceID, AssistantPlanProposed, AssistantPlanAwaitingConfirmation, AssistantPlanExecuting, started)
		if err != nil {
			return record, err
		}
		count, _ := res.RowsAffected()
		if count == 0 {
			return e.store.getAssistantPlan(req.WorkspaceID, record.ID)
		}
		record, err = e.store.getAssistantPlan(req.WorkspaceID, record.ID)
		if err != nil {
			return record, err
		}
	}
	return e.executeRecord(record, req.Actor, leaseToken)
}

func (e *AssistantExecutor) PreviewAssistantPlan(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	return e.Preview(req)
}

func (e *AssistantExecutor) ExecuteAssistantPlan(req AssistantExecuteRequest) (AssistantPlanRecord, error) {
	return e.Execute(req)
}

func (s *Store) getAssistantPlanByIdempotency(workspaceID, key string) (AssistantPlanRecord, error) {
	var id string
	if err := s.db.QueryRow("SELECT id FROM assistant_plans WHERE workspace_id=? AND idempotency_key=?", workspaceID, key).Scan(&id); err != nil {
		return AssistantPlanRecord{}, err
	}
	return s.getAssistantPlan(workspaceID, id)
}

func (s *Store) expireAssistantPlan(record AssistantPlanRecord) (AssistantPlanRecord, error) {
	now := nowString()
	_, err := s.db.Exec("UPDATE assistant_plans SET state=?, error=?, completed_at=?, updated_at=?, lease_until='', lease_token='' WHERE id=? AND workspace_id=? AND state IN (?, ?, ?)", AssistantPlanStale, "confirmation expired", now, now, record.ID, record.WorkspaceID, AssistantPlanProposed, AssistantPlanAwaitingConfirmation, AssistantPlanExecuting)
	if err != nil {
		return record, err
	}
	return s.getAssistantPlan(record.WorkspaceID, record.ID)
}

func (s *Store) finishAssistantPlan(record AssistantPlanRecord, state AssistantPlanState, message, leaseToken string) (AssistantPlanRecord, error) {
	now := nowString()
	result, err := s.db.Exec("UPDATE assistant_plans SET state=?, error=?, completed_at=?, updated_at=?, lease_until='', lease_token='' WHERE id=? AND workspace_id=? AND state=? AND lease_token=?", state, message, now, now, record.ID, record.WorkspaceID, AssistantPlanExecuting, leaseToken)
	if err != nil {
		return record, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return s.getAssistantPlan(record.WorkspaceID, record.ID)
	}
	return s.getAssistantPlan(record.WorkspaceID, record.ID)
}

func (s *Store) claimAssistantAction(workspaceID, planID, actionID, leaseToken string) (bool, error) {
	now := nowString()
	result, err := s.db.Exec(`UPDATE assistant_plan_actions SET state=?, lease_token=?, lease_until=?, started_at=?
WHERE workspace_id=? AND plan_id=? AND action_id=? AND state=? AND EXISTS
(SELECT 1 FROM assistant_plans p WHERE p.id=? AND p.workspace_id=? AND p.state=? AND p.lease_token=? AND p.lease_until>?)`, AssistantActionExecuting, leaseToken, time.Now().UTC().Add(assistantExecutionLeaseTTL).Format(time.RFC3339Nano), now, workspaceID, planID, actionID, AssistantActionPending, planID, workspaceID, AssistantPlanExecuting, leaseToken, now)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (s *Store) leaseValid(workspaceID, planID, leaseToken string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM assistant_plans WHERE id=? AND workspace_id=? AND state=? AND lease_token=? AND lease_until>?", planID, workspaceID, AssistantPlanExecuting, leaseToken, nowString()).Scan(&count)
	return count == 1, err
}

func (e *AssistantExecutor) executeRecord(record AssistantPlanRecord, actor, leaseToken string) (AssistantPlanRecord, error) {
	succeeded, failed := 0, 0
	conflicts := 0
	for i, action := range record.Plan.Actions {
		if i < len(record.Actions) && record.Actions[i].State != "" && record.Actions[i].State != AssistantActionPending && record.Actions[i].State != AssistantActionExecuting {
			if record.Actions[i].State == AssistantActionSucceeded {
				succeeded++
			} else {
				failed++
				if record.Actions[i].State == AssistantActionConflict {
					conflicts++
				}
			}
			continue
		}
		claimed, claimErr := e.store.claimAssistantAction(record.WorkspaceID, record.ID, action.ActionID, leaseToken)
		if claimErr != nil {
			return e.store.getAssistantPlan(record.WorkspaceID, record.ID)
		}
		if !claimed {
			return e.store.finishAssistantPlan(record, AssistantPlanStale, "action was claimed by an expired worker; manual recovery required", leaseToken)
		}
		if beforeAssistantActionHook != nil {
			beforeAssistantActionHook()
		}
		valid, validErr := e.store.leaseValid(record.WorkspaceID, record.ID, leaseToken)
		if validErr != nil || !valid {
			return e.store.finishAssistantPlan(record, AssistantPlanStale, "execution lease expired before mutation", leaseToken)
		}
		result := AssistantActionResult{ActionID: action.ActionID, Operation: action.Operation, State: AssistantActionPending, Actor: actor, ConversationID: record.ConversationID, TurnID: record.TurnID, StartedAt: nowString()}
		value, before, after, eventIDs, err := e.executeAction(record.WorkspaceID, action)
		if afterAssistantActionMutationHook != nil && err == nil {
			afterAssistantActionMutationHook()
		}
		if err != nil {
			result.State = AssistantActionFailed
			if errors.Is(err, errConflict) || strings.Contains(strings.ToLower(err.Error()), "stale version") {
				result.State = AssistantActionConflict
				conflicts++
			}
			result.Error = err.Error()
			failed++
		} else {
			result.State = AssistantActionSucceeded
			succeeded++
			result.BeforeSnapshot = before
			result.AfterSnapshot = after
			result.EventIDs = eventIDs
			if value != nil {
				result.Result, _ = json.Marshal(value)
			}
		}
		result.CompletedAt = nowString()
		if err := e.store.saveAssistantActionResult(record.WorkspaceID, record.ID, i, result, leaseToken); err != nil {
			_, _ = e.store.db.Exec("UPDATE assistant_plans SET error=?, updated_at=? WHERE id=? AND workspace_id=? AND state=? AND lease_token=?", "action result persistence failed after mutation: "+err.Error(), nowString(), record.ID, record.WorkspaceID, AssistantPlanExecuting, leaseToken)
			return e.store.getAssistantPlan(record.WorkspaceID, record.ID)
		}
		if i < len(record.Actions) {
			record.Actions[i] = result
		} else {
			record.Actions = append(record.Actions, result)
		}
	}
	state := AssistantPlanCompleted
	message := ""
	if failed > 0 {
		state = AssistantPlanPartiallyFailed
		if conflicts > 0 && succeeded == 0 {
			state = AssistantPlanStale
		}
		message = fmt.Sprintf("%d of %d actions failed", failed, succeeded+failed)
	}
	return e.store.finishAssistantPlan(record, state, message, leaseToken)
}

func (s *Store) saveAssistantActionResult(workspaceID, planID string, ordinal int, result AssistantActionResult, leaseToken string) error {
	events, _ := json.Marshal(result.EventIDs)
	res, err := s.db.Exec(`UPDATE assistant_plan_actions SET state=?, error=?, before_json=?, after_json=?, result_json=?, event_ids_json=?, actor=?, started_at=?, completed_at=?, lease_until='' WHERE plan_id=? AND workspace_id=? AND ordinal=? AND state=? AND lease_token=? AND EXISTS (SELECT 1 FROM assistant_plans p WHERE p.id=? AND p.workspace_id=? AND p.state=? AND p.lease_token=? AND p.lease_until>?)`, result.State, result.Error, string(result.BeforeSnapshot), string(result.AfterSnapshot), string(result.Result), string(events), result.Actor, result.StartedAt, result.CompletedAt, planID, workspaceID, ordinal, AssistantActionExecuting, leaseToken, planID, workspaceID, AssistantPlanExecuting, leaseToken, nowString())
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return errors.New("assistant action result row was not found")
	}
	return nil
}

func (s *Store) cancelAssistantPlan(workspaceID, planID string) (AssistantPlanRecord, error) {
	if err := s.ensureAssistantWorkspace(workspaceID); err != nil {
		return AssistantPlanRecord{}, err
	}
	now := nowString()
	_, err := s.db.Exec("UPDATE assistant_plans SET state=?, updated_at=?, completed_at=?, error=? WHERE id=? AND workspace_id=? AND state IN (?, ?)", AssistantPlanCancelled, now, now, "cancelled before execution", planID, workspaceID, AssistantPlanProposed, AssistantPlanAwaitingConfirmation)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	return s.getAssistantPlan(workspaceID, planID)
}

func (e *AssistantExecutor) Cancel(workspaceID, planID string) (AssistantPlanRecord, error) {
	return e.store.cancelAssistantPlan(workspaceID, planID)
}

// Undo reverses a completed single-item assistant action only when the target
// still has the exact version produced by that action. This prevents undoing
// over a later manual or assistant edit.
func (e *AssistantExecutor) Undo(workspaceID, planID string) (AssistantPlanRecord, error) {
	record, err := e.store.getAssistantPlan(workspaceID, planID)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	if record.State != AssistantPlanCompleted || len(record.Actions) != 1 {
		return AssistantPlanRecord{}, errors.New("only completed single-action plans can be undone")
	}
	action := record.Actions[0]
	if action.State != AssistantActionSucceeded {
		return AssistantPlanRecord{}, errors.New("assistant action is not successfully completed")
	}
	if action.Operation == AssistantMergeProjects {
		if err := e.undoProjectMerge(workspaceID, action); err != nil {
			return AssistantPlanRecord{}, err
		}
		return e.store.getAssistantPlan(workspaceID, planID)
	}
	var after Item
	if len(action.AfterSnapshot) == 0 || json.Unmarshal(action.AfterSnapshot, &after) != nil || after.ID == "" {
		return AssistantPlanRecord{}, errors.New("this assistant action does not have an undoable item snapshot")
	}
	current, err := e.store.getItem(after.ID)
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	if current.Version != after.Version {
		return AssistantPlanRecord{}, errors.New("cannot undo because the item changed afterwards")
	}
	if len(action.BeforeSnapshot) == 0 {
		_, err = e.store.archiveItemExpected(current.ID, current.Version)
	} else {
		var before Item
		if err := json.Unmarshal(action.BeforeSnapshot, &before); err != nil {
			return AssistantPlanRecord{}, errors.New("invalid undo snapshot")
		}
		before.Version = current.Version
		before.WorkspaceID = workspaceID
		_, err = e.store.updateItem(before)
	}
	if err != nil {
		return AssistantPlanRecord{}, err
	}
	return e.store.getAssistantPlan(workspaceID, planID)
}

func (e *AssistantExecutor) undoProjectMerge(workspaceID string, action AssistantActionResult) error {
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(action.BeforeSnapshot, &before); err != nil {
		return errors.New("invalid merge undo snapshot")
	}
	if err := json.Unmarshal(action.AfterSnapshot, &after); err != nil {
		return errors.New("invalid merge undo snapshot")
	}
	var sourceBefore, targetBefore Project
	var sourceAfter, targetAfter Project
	var beforeItems, afterItems []Item
	for key, dst := range map[string]any{"source": &sourceBefore, "target": &targetBefore, "items": &beforeItems} {
		if err := json.Unmarshal(before[key], dst); err != nil {
			return errors.New("incomplete merge undo snapshot")
		}
	}
	for key, dst := range map[string]any{"sourceProject": &sourceAfter, "targetProject": &targetAfter, "items": &afterItems} {
		if err := json.Unmarshal(after[key], dst); err != nil {
			return errors.New("incomplete merge undo snapshot")
		}
	}
	currentSource, err := e.store.getProject(sourceAfter.ID)
	if err != nil {
		return err
	}
	currentTarget, err := e.store.getProject(targetAfter.ID)
	if err != nil {
		return err
	}
	if currentSource.Version != sourceAfter.Version || currentTarget.Version != targetAfter.Version {
		return errors.New("cannot undo merge because a project changed afterwards")
	}
	afterByID := map[string]Item{}
	for _, item := range afterItems {
		afterByID[item.ID] = item
	}
	affectedItems := make([]Item, 0, len(beforeItems))
	for _, item := range beforeItems {
		if item.ProjectID == sourceBefore.ID {
			affectedItems = append(affectedItems, item)
		}
	}
	for _, item := range affectedItems {
		current, getErr := e.store.getItem(item.ID)
		if getErr != nil {
			return getErr
		}
		if expected, ok := afterByID[item.ID]; ok && current.Version != expected.Version {
			return errors.New("cannot undo merge because an item changed afterwards")
		}
	}
	for _, item := range affectedItems {
		current, getErr := e.store.getItem(item.ID)
		if getErr != nil {
			return getErr
		}
		item.Version = current.Version
		item.WorkspaceID = workspaceID
		if _, updateErr := e.store.updateItem(item); updateErr != nil {
			return updateErr
		}
	}
	sourceBefore.Version = currentSource.Version
	sourceBefore.WorkspaceID = workspaceID
	targetBefore.Version = currentTarget.Version
	targetBefore.WorkspaceID = workspaceID
	if _, err = e.store.updateProject(sourceBefore); err != nil {
		return err
	}
	if _, err = e.store.updateProject(targetBefore); err != nil {
		return err
	}
	return nil
}

func (s *Store) executeAssistantPlan(workspaceID, planID string, confirmed bool) (AssistantPlanRecord, error) {
	return NewAssistantExecutor(s).Execute(AssistantExecuteRequest{WorkspaceID: workspaceID, PlanID: planID, Confirmed: confirmed})
}

func (s *Store) SaveAssistantPlan(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	return s.saveAssistantPlan(req)
}
func (s *Store) CreateAssistantPlan(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	return s.saveAssistantPlan(req)
}
func (s *Store) GetAssistantPlan(workspaceID, id string) (AssistantPlanRecord, error) {
	return s.getAssistantPlan(workspaceID, id)
}
func (s *Store) CancelAssistantPlan(workspaceID, id string) (AssistantPlanRecord, error) {
	return s.cancelAssistantPlan(workspaceID, id)
}
func (s *Store) PreviewAssistantPlan(req AssistantPlanRequest) (AssistantPlanRecord, error) {
	return s.saveAssistantPlan(req)
}
func (s *Store) ExecuteAssistantPlan(req AssistantExecuteRequest) (AssistantPlanRecord, error) {
	return NewAssistantExecutor(s).Execute(req)
}

func marshalSnapshot(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, _ := json.Marshal(value)
	return data
}

func (e *AssistantExecutor) executeAction(workspaceID string, action AssistantAction) (any, json.RawMessage, json.RawMessage, []string, error) {
	if workspaceID != e.store.defaultWorkspaceID() {
		return nil, nil, nil, nil, errors.New("assistant execution workspace is not active")
	}
	if err := action.ValidateTargets(workspaceID, e.store); err != nil {
		return nil, nil, nil, nil, err
	}
	beforeEvents := map[string]struct{}{}
	beforeProjectEvents := map[string]struct{}{}
	track := func(id string) error {
		events, err := e.store.listItemEvents(id, 10000)
		if err != nil {
			return err
		}
		for _, event := range events {
			beforeEvents[event.ID] = struct{}{}
		}
		return nil
	}
	collect := func(id string) ([]string, error) {
		events, err := e.store.listItemEvents(id, 10000)
		if err != nil {
			return nil, err
		}
		out := []string{}
		for _, event := range events {
			if _, ok := beforeEvents[event.ID]; !ok {
				out = append(out, event.ID)
			}
		}
		return out, nil
	}
	trackProject := func(id string) error {
		events, err := e.store.listProjectEvents(workspaceID, id, 10000)
		if err != nil {
			return err
		}
		for _, event := range events {
			beforeProjectEvents[event.ID] = struct{}{}
		}
		return nil
	}
	collectProject := func(id string) ([]string, error) {
		events, err := e.store.listProjectEvents(workspaceID, id, 10000)
		if err != nil {
			return nil, err
		}
		out := []string{}
		for _, event := range events {
			if _, ok := beforeProjectEvents[event.ID]; !ok {
				out = append(out, event.ID)
			}
		}
		return out, nil
	}
	switch p := action.Payload.(type) {
	case *AssistantListItemsPayload:
		items, err := e.store.listItems(p.Status, 1000)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if p.ProjectID != "" {
			filtered := items[:0]
			for _, item := range items {
				if item.ProjectID == p.ProjectID {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		type listItemSummary struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Status  string `json:"status"`
			Project string `json:"project,omitempty"`
			DueAt   string `json:"dueAt,omitempty"`
		}
		result := struct {
			Count int               `json:"count"`
			Items []listItemSummary `json:"items"`
		}{Count: len(items), Items: make([]listItemSummary, 0, len(items))}
		for _, item := range items {
			result.Items = append(result.Items, listItemSummary{ID: item.ID, Title: item.Title, Status: item.Status, Project: item.Project, DueAt: item.DueAt})
		}
		return result, nil, nil, nil, nil
	case *AssistantCreateItemPayload:
		candidate := candidateFromAssistantFields(p.Item)
		if p.Item.ProjectID != nil && *p.Item.ProjectID != "" {
			project, err := e.store.getProject(*p.Item.ProjectID)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			candidate.Project = project.Name
		}
		items, err := e.store.createItems("", []Candidate{candidate}, candidate.Timezone)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if len(items) != 1 {
			return nil, nil, nil, nil, errors.New("item creation returned no item")
		}
		events, err := e.store.listItemEvents(items[0].ID, 10000)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		eventIDs := make([]string, 0, len(events))
		for _, event := range events {
			eventIDs = append(eventIDs, event.ID)
		}
		return items[0], nil, marshalSnapshot(items[0]), eventIDs, nil
	case *AssistantUpdateItemPayload:
		item, err := e.store.getItem(p.ItemID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := track(item.ID); err != nil {
			return nil, nil, nil, nil, err
		}
		before := marshalSnapshot(item)
		applyAssistantItemFields(&item, p.Changes)
		if p.Changes.ProjectID != nil {
			if *p.Changes.ProjectID == "" {
				item.ProjectID, item.Project = "", ""
			} else {
				project, projectErr := e.store.getProject(*p.Changes.ProjectID)
				if projectErr != nil {
					return nil, before, nil, nil, projectErr
				}
				item.ProjectID, item.Project = project.ID, project.Name
			}
		}
		if action.ExpectedItemVersion != nil {
			item.Version = *action.ExpectedItemVersion
		}
		updated, err := e.store.updateItem(item)
		if err != nil {
			return nil, before, nil, nil, err
		}
		eventIDs, err := collect(item.ID)
		if err != nil {
			return nil, before, nil, nil, err
		}
		return updated, before, marshalSnapshot(updated), eventIDs, nil
	case *AssistantItemTargetPayload:
		item, err := e.store.getItem(p.ItemID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := track(item.ID); err != nil {
			return nil, nil, nil, nil, err
		}
		before := marshalSnapshot(item)
		var updated Item
		expected := 0
		if action.ExpectedItemVersion != nil {
			expected = *action.ExpectedItemVersion
		}
		switch action.Operation {
		case AssistantCompleteItem:
			if action.ExpectedItemVersion != nil {
				updated, err = e.store.completeItemExpected(item.ID, expected)
			} else {
				updated, err = e.store.completeItem(item.ID)
			}
		case AssistantReopenItem:
			if action.ExpectedItemVersion != nil {
				updated, err = e.store.reopenItemExpected(item.ID, expected)
			} else {
				updated, err = e.store.reopenItem(item.ID)
			}
		case AssistantArchiveItem:
			if action.ExpectedItemVersion != nil {
				updated, err = e.store.archiveItemExpected(item.ID, expected)
			} else {
				updated, err = e.store.archiveItem(item.ID)
			}
		}
		eventIDs, collectErr := collect(item.ID)
		if err == nil {
			err = collectErr
		}
		return updated, before, marshalSnapshot(updated), eventIDs, err
	case *AssistantSetReminderPayload:
		item, err := e.store.getItem(p.ItemID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := track(item.ID); err != nil {
			return nil, nil, nil, nil, err
		}
		before := marshalSnapshot(item)
		updated, err := e.store.snoozeItemExpected(item.ID, p.ReminderAt, action.ExpectedItemVersion)
		eventIDs, collectErr := collect(item.ID)
		if err == nil {
			err = collectErr
		}
		return updated, before, marshalSnapshot(updated), eventIDs, err
	case *AssistantCreateProjectPayload:
		project, err := e.store.createProject(projectFromAssistantFields(p.Project))
		if err != nil {
			return nil, nil, nil, nil, err
		}
		events, err := e.store.listProjectEvents(workspaceID, project.ID, 10000)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		eventIDs := make([]string, 0, len(events))
		for _, event := range events {
			eventIDs = append(eventIDs, event.ID)
		}
		return project, nil, marshalSnapshot(project), eventIDs, nil
	case *AssistantUpdateProjectPayload:
		project, err := e.store.getProject(p.ProjectID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		before := marshalSnapshot(project)
		if err := trackProject(project.ID); err != nil {
			return nil, before, nil, nil, err
		}
		projectItemIDs := []string{}
		projectItems, listErr := e.store.listItems("", 10000)
		if listErr != nil {
			return nil, before, nil, nil, listErr
		}
		for _, item := range projectItems {
			if item.ProjectID == project.ID {
				if err := track(item.ID); err != nil {
					return nil, nil, nil, nil, err
				}
				projectItemIDs = append(projectItemIDs, item.ID)
			}
		}
		updatedProject := project
		applyAssistantProjectFields(&updatedProject, p.Changes)
		if action.ExpectedProjectVersion != nil {
			updatedProject.Version = *action.ExpectedProjectVersion
		}
		updated, err := e.store.updateProject(updatedProject)
		eventIDs := []string{}
		if err == nil {
			for _, id := range projectItemIDs {
				ids, collectErr := collect(id)
				if collectErr != nil {
					return nil, before, nil, nil, collectErr
				}
				eventIDs = append(eventIDs, ids...)
			}
			projectIDs, projectEventErr := collectProject(project.ID)
			if projectEventErr != nil {
				return nil, before, nil, nil, projectEventErr
			}
			eventIDs = append(eventIDs, projectIDs...)
		}
		return updated, before, marshalSnapshot(updated), eventIDs, err
	case *AssistantMergeProjectsPayload:
		source, err := e.store.getProject(p.SourceProjectID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		target, err := e.store.getProject(p.TargetProjectID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := trackProject(source.ID); err != nil {
			return nil, nil, nil, nil, err
		}
		if err := trackProject(target.ID); err != nil {
			return nil, nil, nil, nil, err
		}
		items, listErr := e.store.listItems("", 10000)
		if listErr != nil {
			return nil, nil, nil, nil, listErr
		}
		sourceItemIDs := []string{}
		for _, item := range items {
			if item.ProjectID == source.ID {
				if err := track(item.ID); err != nil {
					return nil, nil, nil, nil, err
				}
				sourceItemIDs = append(sourceItemIDs, item.ID)
			}
		}
		before := marshalSnapshot(map[string]any{"source": source, "target": target, "items": items})
		expected := map[string]int{}
		for id, version := range action.ExpectedItemVersions {
			expected[id] = version
		}
		sourceExpected := source.Version
		if p.SourceExpectedVersion != nil {
			sourceExpected = *p.SourceExpectedVersion
		}
		targetExpected := target.Version
		if action.ExpectedProjectVersion != nil {
			targetExpected = *action.ExpectedProjectVersion
		}
		err = e.store.mergeProject(source.ID, target.ID, expected, sourceExpected, targetExpected)
		if err != nil {
			return nil, before, nil, nil, err
		}
		afterTarget, targetErr := e.store.getProject(target.ID)
		if targetErr != nil {
			return nil, before, nil, nil, targetErr
		}
		afterSource, sourceErr := e.store.getProject(source.ID)
		if sourceErr != nil {
			return nil, before, nil, nil, sourceErr
		}
		afterItems, itemsErr := e.store.listItems("", 10000)
		if itemsErr != nil {
			return nil, before, nil, nil, itemsErr
		}
		events := []string{}
		for _, id := range sourceItemIDs {
			ids, collectErr := collect(id)
			if collectErr != nil {
				return nil, before, nil, nil, collectErr
			}
			events = append(events, ids...)
		}
		projectIDs, projectEventErr := collectProject(source.ID)
		if projectEventErr != nil {
			return nil, before, nil, nil, projectEventErr
		}
		events = append(events, projectIDs...)
		targetIDs, targetEventErr := collectProject(target.ID)
		if targetEventErr != nil {
			return nil, before, nil, nil, targetEventErr
		}
		events = append(events, targetIDs...)
		afterSnapshot := map[string]any{"sourceProject": afterSource, "targetProject": afterTarget, "items": afterItems}
		return afterSnapshot, before, marshalSnapshot(afterSnapshot), events, nil
	case *AssistantMoveItemsPayload:
		before := []Item{}
		for _, id := range p.ItemIDs {
			item, err := e.store.getItem(id)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if err := track(id); err != nil {
				return nil, nil, nil, nil, err
			}
			before = append(before, item)
		}
		expectedVersions := map[string]int{}
		for id, version := range p.ExpectedVersions {
			expectedVersions[id] = version
		}
		if action.ExpectedItemVersion != nil {
			for _, id := range p.ItemIDs {
				if _, ok := expectedVersions[id]; !ok {
					expectedVersions[id] = *action.ExpectedItemVersion
				}
			}
		}
		projectVersions := []int{}
		if action.ExpectedProjectVersion != nil {
			projectVersions = append(projectVersions, *action.ExpectedProjectVersion)
		}
		moved, err := e.store.moveItemsToProject(p.ItemIDs, p.TargetProjectID, expectedVersions, projectVersions...)
		if err != nil {
			return nil, marshalSnapshot(before), nil, nil, err
		}
		all := []string{}
		for _, id := range p.ItemIDs {
			ids, collectErr := collect(id)
			if collectErr != nil {
				return nil, marshalSnapshot(before), marshalSnapshot(moved), nil, collectErr
			}
			all = append(all, ids...)
		}
		return moved, marshalSnapshot(before), marshalSnapshot(moved), all, nil
	default:
		return nil, nil, nil, nil, errors.New("unsupported assistant operation")
	}
}

func candidateFromAssistantFields(f AssistantItemFields) Candidate {
	c := Candidate{Type: "task", Timezone: "UTC"}
	if f.Type != nil {
		c.Type = *f.Type
	}
	if f.Title != nil {
		c.Title = *f.Title
	}
	if f.Description != nil {
		c.Description = *f.Description
	}
	if f.StartAt != nil {
		c.StartAt = *f.StartAt
	}
	if f.EndAt != nil {
		c.EndAt = *f.EndAt
	}
	if f.DueAt != nil {
		c.DueAt = *f.DueAt
	}
	if f.ReminderAt != nil {
		c.ReminderAt = *f.ReminderAt
	}
	if f.Timezone != nil {
		c.Timezone = *f.Timezone
	}
	if f.AllDay != nil {
		c.AllDay = *f.AllDay
	}
	if f.RecurrenceRule != nil {
		c.RecurrenceRule = *f.RecurrenceRule
	}
	if f.Priority != nil {
		c.Priority = *f.Priority
	}
	if f.DurationMinutes != nil {
		c.DurationMinutes = *f.DurationMinutes
	}
	if f.AvailableFrom != nil {
		c.AvailableFrom = *f.AvailableFrom
	}
	if f.AvailableUntil != nil {
		c.AvailableUntil = *f.AvailableUntil
	}
	if f.ProjectID != nil {
		c.Project = *f.ProjectID
	}
	if f.Tags != nil {
		c.Tags = *f.Tags
	}
	if f.Location != nil {
		c.Location = *f.Location
	}
	if f.People != nil {
		c.People = *f.People
	}
	return c
}
func applyAssistantItemFields(item *Item, f AssistantItemFields) {
	c := candidateFromAssistantFields(f)
	if f.Type != nil {
		item.Type = c.Type
	}
	if f.Title != nil {
		item.Title = c.Title
	}
	if f.Description != nil {
		item.Description = c.Description
	}
	if f.StartAt != nil {
		item.StartAt = c.StartAt
	}
	if f.EndAt != nil {
		item.EndAt = c.EndAt
	}
	if f.DueAt != nil {
		item.DueAt = c.DueAt
	}
	if f.ReminderAt != nil {
		item.ReminderAt = c.ReminderAt
	}
	if f.Timezone != nil {
		item.Timezone = c.Timezone
	}
	if f.AllDay != nil {
		item.AllDay = c.AllDay
	}
	if f.RecurrenceRule != nil {
		item.RecurrenceRule = c.RecurrenceRule
	}
	if f.Priority != nil {
		item.Priority = c.Priority
	}
	if f.DurationMinutes != nil {
		item.DurationMinutes = c.DurationMinutes
	}
	if f.AvailableFrom != nil {
		item.AvailableFrom = c.AvailableFrom
	}
	if f.AvailableUntil != nil {
		item.AvailableUntil = c.AvailableUntil
	}
	if f.Tags != nil {
		item.Tags = c.Tags
	}
	if f.Location != nil {
		item.Location = c.Location
	}
	if f.People != nil {
		item.People = c.People
	}
	if f.ProjectID != nil {
		item.ProjectID = *f.ProjectID
	}
}
func projectFromAssistantFields(f AssistantProjectFields) Project {
	p := Project{}
	if f.Name != nil {
		p.Name = *f.Name
	}
	if f.Description != nil {
		p.Description = *f.Description
	}
	if f.Status != nil {
		p.Status = *f.Status
	}
	if f.Priority != nil {
		p.Priority = *f.Priority
	}
	if f.Color != nil {
		p.Color = *f.Color
	}
	if f.Goal != nil {
		p.Goal = *f.Goal
	}
	if f.Deadline != nil {
		p.Deadline = *f.Deadline
	}
	if f.ReviewAt != nil {
		p.ReviewAt = *f.ReviewAt
	}
	return p
}
func applyAssistantProjectFields(p *Project, f AssistantProjectFields) {
	q := projectFromAssistantFields(f)
	if f.Name != nil {
		p.Name = q.Name
	}
	if f.Description != nil {
		p.Description = q.Description
	}
	if f.Status != nil {
		p.Status = q.Status
	}
	if f.Priority != nil {
		p.Priority = q.Priority
	}
	if f.Color != nil {
		p.Color = q.Color
	}
	if f.Goal != nil {
		p.Goal = q.Goal
	}
	if f.Deadline != nil {
		p.Deadline = q.Deadline
	}
	if f.ReviewAt != nil {
		p.ReviewAt = q.ReviewAt
	}
}
