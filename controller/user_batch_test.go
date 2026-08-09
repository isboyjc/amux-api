package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type batchManageUsersResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Action string `json:"action"`
		Count  int    `json:"count"`
		IDs    []int  `json:"ids"`
	} `json:"data"`
}

func callBatchManageUsers(t *testing.T, role int, request BatchManageRequest) batchManageUsersResponse {
	t.Helper()

	payload, err := common.Marshal(request)
	if err != nil {
		t.Fatalf("序列化批量用户请求失败: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage/batch", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("role", role)

	BatchManageUsers(ctx)

	var response batchManageUsersResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("解析批量用户响应失败: %v; body=%s", err, recorder.Body.String())
	}
	return response
}

func createBatchManageTestUser(t *testing.T, username string, role int, status int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username,
		Password: "test-password",
		Role:     role,
		Status:   status,
		AffCode:  username + "-aff",
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户 %s 失败: %v", username, err)
	}
	return user
}

func TestBatchManageUsersEnableDisableDelete(t *testing.T) {
	openUserConcurrencyTestDB(t)

	user1 := createBatchManageTestUser(t, "batch-user-1", common.RoleCommonUser, common.UserStatusEnabled)
	user2 := createBatchManageTestUser(t, "batch-user-2", common.RoleCommonUser, common.UserStatusDisabled)

	disableResponse := callBatchManageUsers(t, common.RoleRootUser, BatchManageRequest{
		IDs:    []int{user1.Id, user2.Id, user1.Id},
		Action: "disable",
	})
	if !disableResponse.Success || disableResponse.Data.Count != 2 {
		t.Fatalf("批量禁用响应异常: %+v", disableResponse)
	}

	var disabledUsers []model.User
	if err := model.DB.Where("id IN ?", []int{user1.Id, user2.Id}).Find(&disabledUsers).Error; err != nil {
		t.Fatalf("读取禁用用户失败: %v", err)
	}
	for _, user := range disabledUsers {
		if user.Status != common.UserStatusDisabled {
			t.Fatalf("用户 %d 应已禁用，实际状态 %d", user.Id, user.Status)
		}
	}

	enableResponse := callBatchManageUsers(t, common.RoleRootUser, BatchManageRequest{
		IDs:    []int{user1.Id, user2.Id},
		Action: "enable",
	})
	if !enableResponse.Success || enableResponse.Data.Count != 2 {
		t.Fatalf("批量启用响应异常: %+v", enableResponse)
	}

	var enabledUsers []model.User
	if err := model.DB.Where("id IN ?", []int{user1.Id, user2.Id}).Find(&enabledUsers).Error; err != nil {
		t.Fatalf("读取启用用户失败: %v", err)
	}
	for _, user := range enabledUsers {
		if user.Status != common.UserStatusEnabled {
			t.Fatalf("用户 %d 应已启用，实际状态 %d", user.Id, user.Status)
		}
	}

	deleteResponse := callBatchManageUsers(t, common.RoleRootUser, BatchManageRequest{
		IDs:    []int{user1.Id, user2.Id},
		Action: "delete",
	})
	if !deleteResponse.Success || deleteResponse.Data.Count != 2 {
		t.Fatalf("批量删除响应异常: %+v", deleteResponse)
	}

	var deletedUsers []model.User
	if err := model.DB.Unscoped().Where("id IN ?", []int{user1.Id, user2.Id}).Find(&deletedUsers).Error; err != nil {
		t.Fatalf("读取已删除用户失败: %v", err)
	}
	for _, user := range deletedUsers {
		if !user.DeletedAt.Valid {
			t.Fatalf("用户 %d 应已软删除", user.Id)
		}
	}
}

func TestBatchManageUsersValidatesEntireBatchBeforeMutation(t *testing.T) {
	openUserConcurrencyTestDB(t)

	rootUser := createBatchManageTestUser(t, "batch-root", common.RoleRootUser, common.UserStatusEnabled)
	commonUser := createBatchManageTestUser(t, "batch-common", common.RoleCommonUser, common.UserStatusEnabled)

	response := callBatchManageUsers(t, common.RoleRootUser, BatchManageRequest{
		IDs:    []int{commonUser.Id, rootUser.Id},
		Action: "disable",
	})
	if response.Success {
		t.Fatalf("包含超级管理员的批量禁用应失败: %+v", response)
	}

	var unchanged model.User
	if err := model.DB.First(&unchanged, commonUser.Id).Error; err != nil {
		t.Fatalf("读取普通用户失败: %v", err)
	}
	if unchanged.Status != common.UserStatusEnabled {
		t.Fatalf("校验失败时不得部分修改，普通用户状态实际为 %d", unchanged.Status)
	}
}
