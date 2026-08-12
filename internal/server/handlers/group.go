package handlers

import (
	"net/http"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/dlclark/regexp2"
	"github.com/gin-gonic/gin"
)

func RegisterGroupRoutes() []*router.GroupRouter {
	return []*router.GroupRouter{
		router.NewGroupRouter("/api/v1/group").
			Use(middleware.Auth()).
			Use(middleware.RequireJSON()).
			AddRoute(
				router.NewRoute("/list", http.MethodGet).
					Handle(getGroupList),
			).
			AddRoute(
				router.NewRoute("/create", http.MethodPost).
					Handle(createGroup),
			).
			AddRoute(
				router.NewRoute("/update", http.MethodPost).
					Handle(updateGroup),
			).
			AddRoute(
				router.NewRoute("/delete/:id", http.MethodDelete).
					Handle(deleteGroup),
			),
	}
}

func getGroupList(c *gin.Context) {
	groups, err := op.GroupList(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, groups)
}

func createGroup(c *gin.Context) {
	var group model.Group
	if !bindJSON(c, &group) {
		return
	}
	if group.MatchRegex != "" {
		_, err := regexp2.Compile(group.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := op.GroupCreate(&group, c.Request.Context()); err != nil {
		handleWriteError(c, err)
		return
	}
	resp.Success(c, group)
}

func updateGroup(c *gin.Context) {
	var req model.GroupUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.MatchRegex != nil {
		_, err := regexp2.Compile(*req.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	group, err := op.GroupUpdate(&req, c.Request.Context())
	if err != nil {
		handleWriteError(c, err)
		return
	}
	resp.Success(c, group)
}

func deleteGroup(c *gin.Context) {
	idNum, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := op.GroupDel(idNum, c.Request.Context()); err != nil {
		serverError(c, err)
		return
	}
	resp.Success(c, "group deleted successfully")
}
