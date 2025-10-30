package modules

import (
	"context"
	"encoding/json"
	"evolve/db/connection"
	"evolve/util"
	"fmt"
	"time"
)

type (
	ShareRunReq struct {
		RunID         string   `json:"runID"`
		UserEmailList []string `json:"userEmailList"` // List of emails to share the run with.
	}

	RunDataReq struct {
		RunID string `json:"runID"`
	}
)

func UserRuns(ctx context.Context, userID string, logger *util.LoggerService) ([]map[string]string, error) {
	db, err := connection.PoolConn(ctx)
	if err != nil {
		logger.Error(fmt.Sprintf("UserRuns: %s", err.Error()), err)
		return nil, fmt.Errorf("something went wrong")
	}

	var runIDs []string
	rows, err := db.Query(ctx, "SELECT runID FROM access WHERE userID = $1", userID)
	if err != nil {
		logger.Error(fmt.Sprintf("UserRuns.db.Query: %s", err.Error()), err)
		return nil, fmt.Errorf("something went wrong")
	}

	for rows.Next() {
		var runID string
		err = rows.Scan(&runID)
		if err != nil {
			logger.Error(fmt.Sprintf("UserRuns.rows.Scan: %s", err.Error()), err)
			return nil, fmt.Errorf("something went wrong")
		}
		runIDs = append(runIDs, runID)
	}

	if len(runIDs) == 0 {
		return make([]map[string]string, 0), nil
	}

	// logger.Info(fmt.Sprintf("RunIDs: %s", runIDs))

	rows, err = db.Query(ctx, "SELECT * FROM run WHERE id = ANY($1)", runIDs)
	if err != nil {
		logger.Error(fmt.Sprintf("UserRuns.db.Query: %s", err.Error()), err)
		return nil, fmt.Errorf("something went wrong")
	}

	runs := []map[string]string{}
	for rows.Next() {
		var id string
		var name string
		var description string
		var status string
		var runType string
		var command string
		var createdBy string
		var createdAt time.Time
		var updatedAt time.Time

		err := rows.Scan(&id, &name, &description, &status, &runType, &command, &createdBy, &createdAt, &updatedAt)
		if err != nil {
			logger.Error(fmt.Sprintf("UserRuns.rows.Scan: %s", err.Error()), err)
			return nil, fmt.Errorf("something went wrong")
		}

		run := map[string]string{
			"id":          id,
			"name":        name,
			"description": description,
			"status":      status,
			"type":        runType,
			"command":     command,
			"createdAt":   createdAt.Local().String(),
			"updatedAt":   updatedAt.Local().String(),
		}

		if createdBy != userID {
			run["isShared"] = "true"
			run["sharedBy"] = createdBy
		} else {
			run["isShared"] = "false"
			run["createdBy"] = createdBy
		}

		runs = append(runs, run)
	}

	// logger.Info(fmt.Sprintf("Runs: %s", runs))

	return runs, nil
}

func ShareRunReqFromJSON(jsonData map[string]any) (*ShareRunReq, error) {
	s := &ShareRunReq{}
	jsonDataBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonDataBytes, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ShareRunReq) ShareRun(ctx context.Context, logger *util.LoggerService) error {
	db, err := connection.PoolConn(ctx)
	if err != nil {
		logger.Error(fmt.Sprintf("ShareRun: %s", err.Error()), err)
		return fmt.Errorf("something went wrong")
	}

	// Check if run exists.
	var runID string
	if err := db.QueryRow(ctx, "SELECT id FROM run WHERE id = $1", s.RunID).Scan(&runID); err != nil {
		logger.Error(fmt.Sprintf("ShareRun.db.QueryRow: %s", err.Error()), err)
		return fmt.Errorf("run does not exist")
	}

	// Check if provided emails exist.
	rows, err := db.Query(ctx, "SELECT id FROM users WHERE email = ANY($1)", s.UserEmailList)
	if err != nil {
		logger.Error(fmt.Sprintf("ShareRun.db.Query: %s", err.Error()), err)
		return fmt.Errorf("something went wrong")
	}

	var userIDs []string
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			logger.Error(fmt.Sprintf("ShareRun.rows.Scan: %s", err.Error()), err)
			return fmt.Errorf("something went wrong")
		}
		userIDs = append(userIDs, id)
	}

	if len(userIDs) == 0 || len(userIDs) != len(s.UserEmailList) {
		return fmt.Errorf("please check the emails again. some of them do not exist")
	}

	// Share the run with the users.
	for _, userID := range userIDs {
		_, err = db.Exec(ctx, "INSERT INTO access (runID, userID, mode) VALUES ($1, $2, $3)", s.RunID, userID, "read")
		if err != nil {
			logger.Error(fmt.Sprintf("ShareRun.db.Exec: %s", err.Error()), err)
			return fmt.Errorf("make sure the run is not already shared with the user")
		}
	}

	return nil
}

func RunDataReqFromJSON(jsonData map[string]any) (*RunDataReq, error) {
	r := &RunDataReq{}
	jsonDataBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonDataBytes, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RunDataReq) UserRun(ctx context.Context, userID string, logger *util.LoggerService) (map[string]string, error) {
	db, err := connection.PoolConn(ctx)
	if err != nil {
		logger.Error(fmt.Sprintf("RunData: %s", err.Error()), err)
		return nil, fmt.Errorf("something went wrong")
	}

	// Check if user has access to the run.
	var runID string
	if err := db.QueryRow(ctx, "SELECT runID FROM access WHERE userID = $1 AND runID = $2", userID, r.RunID).Scan(&runID); err != nil {
		logger.Error(fmt.Sprintf("RunData.db.QueryRow: %s", err.Error()), err)
		return nil, fmt.Errorf("run does not exist")
	}

	var id, name, description, status, runType, command, createdBy string
	var createdAt, updatedAt time.Time
	// Get the run details like name, description, status, type, command, createdBy, createdAt, updatedAt.
	err = db.QueryRow(ctx, "SELECT id, name, description, status, type, command, createdBy, createdAt, updatedAt FROM run WHERE id = $1", r.RunID).Scan(&id, &name, &description, &status, &runType, &command, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		logger.Error(fmt.Sprintf("RunData.db.QueryRow: %s", err.Error()), err)
		return nil, fmt.Errorf("something went wrong")
	}

	return map[string]string{
		"id":          id,
		"name":        name,
		"description": description,
		"status":      status,
		"type":        runType,
		"command":     command,
		"createdBy":   createdBy,
		"createdAt":   createdAt.Local().String(),
		"updatedAt":   updatedAt.Local().String(),
	}, nil
}
