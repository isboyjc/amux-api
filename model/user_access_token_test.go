package model

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ensureUatSchema 在每个用例开头调，确保表已建（TestMain 在另一个文件，不动它，
// 直接在这里幂等保证 schema 即可）。同时清表防止用例间脏数据。
func ensureUatSchema(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&UserAccessToken{}, &User{}))
	DB.Exec("DELETE FROM user_access_tokens")
	DB.Exec("DELETE FROM users")
}

// ============= token 形态 =============

func TestGenerateAccessTokenString_FormatPAT(t *testing.T) {
	plaintext, prefix, hash, err := GenerateAccessTokenString(UserAccessTokenPrefixPAT)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plaintext, UserAccessTokenPrefixPAT),
		"plaintext should start with PAT prefix")
	require.Equal(t, UserAccessTokenPrefixStoreLen, len(prefix))
	require.Equal(t, 64, len(hash), "sha256 hex must be 64 chars")
	require.Equal(t, len(UserAccessTokenPrefixPAT)+UserAccessTokenRandomLen, len(plaintext))

	// 字符集校验
	for _, c := range plaintext[len(UserAccessTokenPrefixPAT):] {
		require.True(t,
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'),
			"non-base62 char: %c", c)
	}
}

func TestGenerateAccessTokenString_FormatOAT(t *testing.T) {
	plaintext, _, _, err := GenerateAccessTokenString(UserAccessTokenPrefixOAT)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plaintext, UserAccessTokenPrefixOAT))
}

func TestHashAccessToken_Deterministic(t *testing.T) {
	h1 := HashAccessToken("amux_api_pat_xxxx")
	h2 := HashAccessToken("amux_api_pat_xxxx")
	require.Equal(t, h1, h2)
	require.NotEqual(t, h1, HashAccessToken("amux_api_pat_yyyy"))
}

func TestConstantTimeMatchHash(t *testing.T) {
	require.True(t, ConstantTimeMatchHash("abc", "abc"))
	require.False(t, ConstantTimeMatchHash("abc", "abd"))
	require.False(t, ConstantTimeMatchHash("abc", "abcd"), "different lengths must not match")
}

// ============= CRUD =============

func TestCreateAndValidateUserAccessToken(t *testing.T) {
	ensureUatSchema(t)

	// 建一个 user
	u := &User{Username: "alice", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	rec := &UserAccessToken{
		UserId: u.Id,
		Name:   "My CLI",
	}
	plaintext, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plaintext, UserAccessTokenPrefixPAT))
	require.NotZero(t, rec.Id)
	require.Equal(t, UserAccessTokenStatusActive, rec.Status)

	// 校验路径
	user, matched, err := ValidateUserAccessToken(plaintext, "1.2.3.4")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, u.Id, user.Id)
	require.NotNil(t, matched)
	require.Equal(t, rec.Id, matched.Id)
}

func TestValidateUserAccessToken_RejectsWrongPrefix(t *testing.T) {
	ensureUatSchema(t)
	user, matched, err := ValidateUserAccessToken("not_amux_xxxx", "")
	require.NoError(t, err)
	require.Nil(t, user, "non-amux_api prefix must skip new-table path")
	require.Nil(t, matched)
}

func TestValidateUserAccessToken_Revoked(t *testing.T) {
	ensureUatSchema(t)
	u := &User{Username: "bob", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	rec := &UserAccessToken{UserId: u.Id, Name: "X"}
	plaintext, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
	require.NoError(t, err)

	// 撤销
	require.NoError(t, RevokeUserAccessToken(rec.Id, UserAccessTokenRevokeUser))

	user, matched, err := ValidateUserAccessToken(plaintext, "")
	require.NoError(t, err)
	require.Nil(t, user, "revoked token must not validate")
	require.Nil(t, matched)
}

func TestValidateUserAccessToken_Expired(t *testing.T) {
	ensureUatSchema(t)
	u := &User{Username: "carol", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	past := time.Now().Unix() - 60
	rec := &UserAccessToken{UserId: u.Id, Name: "X", ExpiresAt: &past}
	plaintext, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
	require.NoError(t, err)

	user, _, err := ValidateUserAccessToken(plaintext, "")
	require.NoError(t, err)
	require.Nil(t, user, "expired token must not validate")
}

func TestRotateUserAccessToken(t *testing.T) {
	ensureUatSchema(t)
	u := &User{Username: "dave", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	rec := &UserAccessToken{UserId: u.Id, Name: "X", Description: "desc"}
	oldPlain, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
	require.NoError(t, err)

	fresh, newPlain, err := RotateUserAccessToken(rec.Id)
	require.NoError(t, err)
	require.NotEqual(t, oldPlain, newPlain, "rotated token plaintext must differ")
	require.Equal(t, "desc", fresh.Description, "metadata preserved")

	// 旧 token 不再校验通过
	user, _, err := ValidateUserAccessToken(oldPlain, "")
	require.NoError(t, err)
	require.Nil(t, user, "old token must be invalidated after rotate")

	// 新 token 校验通过
	user2, _, err := ValidateUserAccessToken(newPlain, "")
	require.NoError(t, err)
	require.NotNil(t, user2)

	// 旧记录状态 = revoked + reason = rotate
	var old UserAccessToken
	require.NoError(t, DB.Where("id = ?", rec.Id).First(&old).Error)
	assert.Equal(t, UserAccessTokenStatusRevoked, old.Status)
	assert.Equal(t, UserAccessTokenRevokeRotate, old.RevokeReason)
}

func TestRevokeAllUserAccessTokens_BySource(t *testing.T) {
	ensureUatSchema(t)
	u := &User{Username: "eve", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	for _, src := range []string{
		UserAccessTokenSourceManual,
		UserAccessTokenSourceLegacy,
		UserAccessTokenSourceDeviceFlow,
	} {
		rec := &UserAccessToken{UserId: u.Id, Name: src, Source: src}
		_, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
		require.NoError(t, err)
	}

	// 仅撤 legacy
	require.NoError(t, RevokeAllUserAccessTokens(u.Id,
		UserAccessTokenRevokePasswordReset, UserAccessTokenSourceLegacy))

	tokens, err := ListUserAccessTokensByUser(u.Id, 0)
	require.NoError(t, err)
	require.Len(t, tokens, 3)
	for _, tk := range tokens {
		if tk.Source == UserAccessTokenSourceLegacy {
			assert.Equal(t, UserAccessTokenStatusRevoked, tk.Status)
		} else {
			assert.Equal(t, UserAccessTokenStatusActive, tk.Status)
		}
	}
}

// ============= 数据迁移 =============

func TestMigrateLegacyAccessTokens(t *testing.T) {
	ensureUatSchema(t)
	require.NoError(t, DB.AutoMigrate(&User{}))

	legacy := "abcdefghij1234567890ZYXWVUTSrqpO" // 32 字符
	tok := legacy
	u := &User{Username: "frank", Password: "hash", AccessToken: &tok}
	require.NoError(t, DB.Create(u).Error)

	// 重置 once，否则在同一进程里多个用例只迁一次
	migrateLegacyOnce = sync.Once{}
	require.NoError(t, MigrateLegacyAccessTokens())

	tokens, err := ListUserAccessTokensByUser(u.Id, 0)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, UserAccessTokenSourceLegacy, tokens[0].Source)
	assert.Equal(t, "Imported", tokens[0].Name)
	assert.Equal(t, HashAccessToken(legacy), tokens[0].TokenHash)

	// 重复迁移应幂等
	migrateLegacyOnce = sync.Once{}
	require.NoError(t, MigrateLegacyAccessTokens())
	tokens2, _ := ListUserAccessTokensByUser(u.Id, 0)
	assert.Len(t, tokens2, 1, "second migration must be idempotent")
}

// ============= 兼容路径：旧 32 字符 token =============

func TestValidateAccessToken_LegacyFallback(t *testing.T) {
	ensureUatSchema(t)
	require.NoError(t, DB.AutoMigrate(&User{}))

	legacy := "abcdefghij1234567890ZYXWVUTSrqpO"
	tok := legacy
	u := &User{Username: "grace", Password: "hash", AccessToken: &tok}
	require.NoError(t, DB.Create(u).Error)

	// 不带 amux_api_ 前缀 → 走 fallback
	user, matched, err := ValidateAccessTokenWithIP(legacy, "")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, u.Id, user.Id)
	assert.Nil(t, matched, "legacy fallback returns nil tokenRecord")
}

// ============= 空闲清理 =============

func TestCleanupExpiredUserAccessTokens_HardExpiry(t *testing.T) {
	ensureUatSchema(t)
	u := &User{Username: "hank", Password: "hash"}
	require.NoError(t, DB.Create(u).Error)

	past := time.Now().Unix() - 60
	rec := &UserAccessToken{UserId: u.Id, Name: "expired-soon", ExpiresAt: &past}
	_, err := CreateUserAccessToken(rec, UserAccessTokenPrefixPAT)
	require.NoError(t, err)

	require.NoError(t, CleanupExpiredUserAccessTokens())

	var got UserAccessToken
	require.NoError(t, DB.Where("id = ?", rec.Id).First(&got).Error)
	assert.Equal(t, UserAccessTokenStatusExpired, got.Status)
}
