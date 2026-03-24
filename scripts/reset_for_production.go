package main

import (
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// 初始化环境
	_ = godotenv.Load(".env")
	common.InitEnv()
	logger.SetupLogger()
	
	common.SysLog("生产环境重置脚本启动")
	
	// 初始化数据库
	err := model.InitDB()
	if err != nil {
		common.FatalLog("数据库初始化失败: " + err.Error())
		os.Exit(1)
	}
	defer model.CloseDB()
	
	// 初始化日志数据库
	err = model.InitLogDB()
	if err != nil {
		common.FatalLog("日志数据库初始化失败: " + err.Error())
		os.Exit(1)
	}

	// 计算 $5 对应的 quota 值
	resetQuota := int(5 * common.QuotaPerUnit)

	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║           生产环境数据重置脚本                              ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("本脚本将执行以下操作:")
	fmt.Println()
	fmt.Println("1. 硬删除已注销用户及其关联数据")
	fmt.Println("   - 用户表中 DeletedAt 不为空的用户")
	fmt.Println("   - 关联的 Token、OAuth绑定、2FA、Passkey 等")
	fmt.Println()
	fmt.Println("2. 重置普通用户额度（排除管理员 role >= 10）")
	fmt.Printf("   - 剩余额度(Quota): $5 (%d)\n", resetQuota)
	fmt.Println("   - 已使用额度(UsedQuota): 0")
	fmt.Println("   - 请求次数(RequestCount): 0")
	fmt.Println("   - 邀请待使用收益(AffQuota): 0")
	fmt.Println("   - 邀请历史总收益(AffHistoryQuota): 0")
	fmt.Println("   - 邀请人数(AffCount): 保持不变")
	fmt.Println()
	fmt.Println("3. 删除所有用户的 Token（API令牌）")
	fmt.Println("   - 清空所有 Token 表数据")
	fmt.Println()
	fmt.Println("4. 清空测试数据")
	fmt.Println("   - 日志记录 (Log)")
	fmt.Println("   - 充值记录 (TopUp)")
	fmt.Println("   - 兑换码使用记录 (Redemption - 仅已使用的)")
	fmt.Println("   - 新任务记录 (Task - Suno等)")
	fmt.Println("   - MJ任务记录 (Midjourney)")
	fmt.Println("   - 签到记录 (Checkin)")
	fmt.Println("   - 数据统计 (QuotaData)")
	fmt.Println("   - 订阅订单和记录 (SubscriptionOrder, UserSubscription)")
	fmt.Println()
	fmt.Println("⚠️  警告: 此操作不可逆，请确保已备份数据库！")
	fmt.Println()
	fmt.Print("确认执行此操作？输入 'yes' 继续: ")
	
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Println("操作已取消")
		return
	}

	fmt.Println()
	fmt.Println("开始执行重置操作...")
	fmt.Println()

	// 统计信息
	var deletedUserCount int64
	var affectedUserCount int64
	var deletedLogCount int64
	var deletedTopUpCount int64
	var deletedRedemptionCount int64
	var deletedTaskCount int64
	var deletedCheckinCount int64
	var deletedQuotaDataCount int64
	var deletedSubscriptionOrderCount int64
	var deletedUserSubscriptionCount int64
	var result *gorm.DB

	// ============================================================
	// 步骤1: 获取已注销用户列表
	// ============================================================
	fmt.Println("📋 步骤 1/10: 查找已注销用户...")
	var deletedUsers []model.User
	if err := model.DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&deletedUsers).Error; err != nil {
		common.SysLog("获取已注销用户失败: " + err.Error())
		fmt.Printf("❌ 错误: %v\n", err)
		os.Exit(1)
	}

	deletedUserIds := make([]int, 0)
	for _, user := range deletedUsers {
		deletedUserIds = append(deletedUserIds, user.Id)
	}
	deletedUserCount = int64(len(deletedUserIds))

	if deletedUserCount > 0 {
		fmt.Printf("   找到 %d 个已注销用户\n", deletedUserCount)
	} else {
		fmt.Println("   没有找到已注销用户")
	}

	// ============================================================
	// 步骤2: 删除已注销用户的关联数据
	// ============================================================
	if deletedUserCount > 0 {
		fmt.Println("\n📋 步骤 2/10: 删除已注销用户的关联数据...")

		// 注意：Token 会在步骤5统一删除，这里跳过

		// 删除 UserOAuthBinding
		result = model.DB.Unscoped().Where("user_id IN ?", deletedUserIds).Delete(&model.UserOAuthBinding{})
		if result.Error != nil {
			fmt.Printf("   ⚠️  删除 OAuth 绑定失败: %v\n", result.Error)
		} else {
			fmt.Printf("   ✓ 删除 %d 个 OAuth 绑定\n", result.RowsAffected)
		}

		// 删除 TwoFA
		result = model.DB.Unscoped().Where("user_id IN ?", deletedUserIds).Delete(&model.TwoFA{})
		if result.Error != nil {
			fmt.Printf("   ⚠️  删除 2FA 记录失败: %v\n", result.Error)
		} else {
			fmt.Printf("   ✓ 删除 %d 个 2FA 记录\n", result.RowsAffected)
		}

		// 删除 TwoFABackupCode
		result = model.DB.Unscoped().Where("user_id IN ?", deletedUserIds).Delete(&model.TwoFABackupCode{})
		if result.Error != nil {
			fmt.Printf("   ⚠️  删除 2FA 备用码失败: %v\n", result.Error)
		} else {
			fmt.Printf("   ✓ 删除 %d 个 2FA 备用码\n", result.RowsAffected)
		}

		// 删除 PasskeyCredential
		result = model.DB.Unscoped().Where("user_id IN ?", deletedUserIds).Delete(&model.PasskeyCredential{})
		if result.Error != nil {
			fmt.Printf("   ⚠️  删除 Passkey 失败: %v\n", result.Error)
		} else {
			fmt.Printf("   ✓ 删除 %d 个 Passkey\n", result.RowsAffected)
		}

		// 硬删除用户
		fmt.Println("\n📋 步骤 3/10: 永久删除已注销用户...")
		result = model.DB.Unscoped().Where("id IN ?", deletedUserIds).Delete(&model.User{})
		if result.Error != nil {
			common.SysLog("硬删除用户失败: " + result.Error.Error())
			fmt.Printf("❌ 错误: %v\n", result.Error)
			os.Exit(1)
		}
		fmt.Printf("   ✓ 永久删除 %d 个用户\n", result.RowsAffected)
	} else {
		fmt.Println("\n📋 步骤 2/10: 跳过（无已注销用户）")
		fmt.Println("\n📋 步骤 3/10: 跳过（无已注销用户）")
	}

	// ============================================================
	// 步骤4: 重置普通用户额度（排除管理员）
	// ============================================================
	fmt.Println("\n📋 步骤 4/10: 重置普通用户额度...")
	result = model.DB.Model(&model.User{}).
		Where("role < ?", common.RoleAdminUser). // 只重置普通用户
		Updates(map[string]interface{}{
			"quota":         resetQuota,
			"used_quota":    0,
			"request_count": 0,
			"aff_quota":     0,
			"aff_history":   0, // 注意：数据库列名是 aff_history，不是 aff_history_quota
		})

	if result.Error != nil {
		common.SysLog("重置用户额度失败: " + result.Error.Error())
		fmt.Printf("❌ 错误: %v\n", result.Error)
		os.Exit(1)
	}
	affectedUserCount = result.RowsAffected
	fmt.Printf("   ✓ 成功重置 %d 个普通用户的额度\n", affectedUserCount)

	// ============================================================
	// 步骤5: 删除所有用户的 Token
	// ============================================================
	fmt.Println("\n📋 步骤 5/10: 删除所有用户的 Token...")
	var allTokenCount int64
	model.DB.Model(&model.Token{}).Count(&allTokenCount)
	
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.Token{})

	if result.Error != nil {
		fmt.Printf("   ⚠️  删除 Token 失败: %v\n", result.Error)
	} else {
		fmt.Printf("   ✓ 成功删除 %d 个 Token\n", allTokenCount)
	}

	// ============================================================
	// 步骤6: 清空日志记录
	// ============================================================
	fmt.Println("\n📋 步骤 6/10: 清空日志记录...")
	result = model.LOG_DB.Unscoped().Where("1 = 1").Delete(&model.Log{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空日志失败: %v\n", result.Error)
	} else {
		deletedLogCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条日志\n", deletedLogCount)
	}

	// ============================================================
	// 步骤7: 清空充值记录
	// ============================================================
	fmt.Println("\n📋 步骤 7/10: 清空充值记录...")
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.TopUp{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空充值记录失败: %v\n", result.Error)
	} else {
		deletedTopUpCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条充值记录\n", deletedTopUpCount)
	}

	// ============================================================
	// 步骤8: 重置已使用的兑换码
	// ============================================================
	fmt.Println("\n📋 步骤 8/10: 重置已使用的兑换码...")
	result = model.DB.Model(&model.Redemption{}).
		Where("used_user_id > 0").
		Updates(map[string]interface{}{
			"used_user_id":  0,
			"redeemed_time": 0,
			"status":        1, // 1 = enabled
		})
	if result.Error != nil {
		fmt.Printf("   ⚠️  重置兑换码失败: %v\n", result.Error)
	} else {
		deletedRedemptionCount = result.RowsAffected
		fmt.Printf("   ✓ 成功重置 %d 个兑换码\n", deletedRedemptionCount)
	}

	// ============================================================
	// 步骤9: 清空任务记录（Midjourney/Suno等）
	// ============================================================
	fmt.Println("\n📋 步骤 9/10: 清空任务记录...")
	
	// 清空 Task 表（Suno等新任务）
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.Task{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空 Task 记录失败: %v\n", result.Error)
	} else {
		deletedTaskCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条 Task 记录\n", deletedTaskCount)
	}
	
	// 清空 Midjourney 表
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.Midjourney{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空 Midjourney 记录失败: %v\n", result.Error)
	} else {
		fmt.Printf("   ✓ 成功删除 %d 条 Midjourney 记录\n", result.RowsAffected)
		deletedTaskCount += result.RowsAffected
	}

	// ============================================================
	// 步骤10: 清空其他测试数据
	// ============================================================
	fmt.Println("\n📋 步骤 10/10: 清空其他测试数据...")

	// 清空签到记录
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.Checkin{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空签到记录失败: %v\n", result.Error)
	} else {
		deletedCheckinCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条签到记录\n", deletedCheckinCount)
	}

	// 清空数据统计
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.QuotaData{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空数据统计失败: %v\n", result.Error)
	} else {
		deletedQuotaDataCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条数据统计\n", deletedQuotaDataCount)
	}

	// 清空订阅订单
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.SubscriptionOrder{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空订阅订单失败: %v\n", result.Error)
	} else {
		deletedSubscriptionOrderCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条订阅订单\n", deletedSubscriptionOrderCount)
	}

	// 清空用户订阅
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.UserSubscription{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空用户订阅失败: %v\n", result.Error)
	} else {
		deletedUserSubscriptionCount = result.RowsAffected
		fmt.Printf("   ✓ 成功删除 %d 条用户订阅\n", deletedUserSubscriptionCount)
	}

	// 清空订阅预消费记录
	result = model.DB.Unscoped().Where("1 = 1").Delete(&model.SubscriptionPreConsumeRecord{})
	if result.Error != nil {
		fmt.Printf("   ⚠️  清空订阅预消费记录失败: %v\n", result.Error)
	} else {
		fmt.Printf("   ✓ 成功删除 %d 条订阅预消费记录\n", result.RowsAffected)
	}

	// ============================================================
	// 汇总报告
	// ============================================================
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║                    执行结果汇总                            ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("✓ 永久删除已注销用户: %d 个\n", deletedUserCount)
	fmt.Printf("✓ 重置普通用户额度: %d 个\n", affectedUserCount)
	fmt.Printf("✓ 删除所有 Token: 已清空\n")
	fmt.Printf("✓ 删除日志记录: %d 条\n", deletedLogCount)
	fmt.Printf("✓ 删除充值记录: %d 条\n", deletedTopUpCount)
	fmt.Printf("✓ 重置兑换码: %d 个\n", deletedRedemptionCount)
	fmt.Printf("✓ 删除任务记录: %d 条\n", deletedTaskCount)
	fmt.Printf("✓ 删除签到记录: %d 条\n", deletedCheckinCount)
	fmt.Printf("✓ 删除数据统计: %d 条\n", deletedQuotaDataCount)
	fmt.Printf("✓ 删除订阅订单: %d 条\n", deletedSubscriptionOrderCount)
	fmt.Printf("✓ 删除用户订阅: %d 条\n", deletedUserSubscriptionCount)
	fmt.Println()
	fmt.Println("🎉 重置完成！")
	fmt.Println()
	fmt.Println("⚠️  重要提示:")
	fmt.Println("   1. 请重启服务以清除内存和 Redis 缓存")
	fmt.Println("   2. 建议检查数据库，确认重置结果符合预期")
	fmt.Println("   3. 如有问题，请使用备份恢复")
	fmt.Println()

	common.SysLog(fmt.Sprintf("生产环境重置完成: 删除%d个已注销用户, 重置%d个普通用户",
		deletedUserCount, affectedUserCount))
}
