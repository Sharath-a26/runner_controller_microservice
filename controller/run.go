package controller

import (
	"evolve/modules"
	"evolve/util"
	"fmt"
	"net/http"
)

func UserRun(res http.ResponseWriter, req *http.Request) {
	var logger = util.SharedLogger
	logger.InfoCtx(req, "UserRuns API called.")

	user, err := modules.Auth(req)
	if err != nil {
		util.JSONResponse(res, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	// User has id, role, userName, email & fullName.
	logger.InfoCtx(req, fmt.Sprintf("User: %s", user))

	data, err := util.Body(req)
	if err != nil {
		util.JSONResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}

	run, err := modules.RunDataReqFromJSON(data)
	if err != nil {
		util.JSONResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}

	logger.InfoCtx(req, fmt.Sprintf("Run: %s", run.RunID))

	runData, err := run.UserRun(req.Context(), user["id"], logger)
	if err != nil {
		util.JSONResponse(res, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	util.JSONResponse(res, http.StatusOK, "User run", runData)
}

func UserRuns(res http.ResponseWriter, req *http.Request) {
	var logger = util.SharedLogger
	logger.InfoCtx(req, "UserRuns API called.")

	user, err := modules.Auth(req)
	if err != nil {
		util.JSONResponse(res, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	// User has id, role, userName, email & fullName.
	logger.InfoCtx(req, fmt.Sprintf("User: %s", user))

	runs, err := modules.UserRuns(req.Context(), user["id"], logger)
	if err != nil {
		util.JSONResponse(res, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	util.JSONResponse(res, http.StatusOK, "User runs", runs)
}

func ShareRun(res http.ResponseWriter, req *http.Request) {
	var logger = util.SharedLogger
	logger.InfoCtx(req, "ShareRun API called.")

	user, err := modules.Auth(req)
	if err != nil {
		util.JSONResponse(res, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	// User has id, role, userName, email & fullName.
	logger.InfoCtx(req, fmt.Sprintf("User: %s", user))

	data, err := util.Body(req)
	if err != nil {
		util.JSONResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}

	srq, err := modules.ShareRunReqFromJSON(data)
	if err != nil {
		util.JSONResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}

	if err := srq.ShareRun(req.Context(), logger); err != nil {
		util.JSONResponse(res, http.StatusBadRequest, err.Error(), nil)
		return
	}

	util.JSONResponse(res, http.StatusOK, "Run shared.", nil)
}
