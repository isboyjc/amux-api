/*
Copyright (C) 2025 QuantumNous

Licensed under AGPL-3.0. See repository LICENSE for details.
*/

// 工单 type / category / status / role 的可视化映射。
//
// 关键设计点（与 i18n 相关）：
//   i18next-cli 静态扫描器无法解析 t(<变量>)，只认 t('字面量')。所以这里
//   把所有需要翻译的中文源串都写成"t('字面量')"形式，作为函数被调用时
//   显式 switch—— 字面量留在源码里，extractor 才能采集到 key。
//   如果未来要新增分类，记得在对应 switch 里加一条 t('xxx')，否则 en/ja
//   等语种里就会出现"未抓到 key → 显示中文兜底"的情况。

// 颜色不需翻译，独立保留。
export const STATUS_COLOR = {
  0: 'blue',
  1: 'orange',
  2: 'green',
  3: 'grey',
};

// 优先级 Tag 颜色：低=灰，普通=白(默认)，高=橙，紧急=红
export const PRIORITY_COLOR = {
  0: 'grey',
  1: 'white',
  2: 'orange',
  3: 'red',
};

export const ROLE_COLOR = {
  0: 'blue',
  1: 'green',
  2: 'grey',
};

export function tType(t, type) {
  switch (type) {
    case 'support':
      return t('求助');
    case 'feedback':
      return t('反馈/建议');
    default:
      return type;
  }
}

export function tCategory(t, category) {
  switch (category) {
    case 'model_invocation':
      return t('模型调用问题');
    case 'channel_issue':
      return t('渠道/分组问题');
    case 'billing':
      return t('计费/额度');
    case 'account':
      return t('账号/鉴权');
    case 'abuse':
      return t('滥用举报');
    case 'refund':
      return t('退款申请');
    case 'feature':
      return t('功能建议');
    case 'ux':
      return t('界面/体验');
    case 'docs':
      return t('文档建议');
    case 'other':
      return t('其他');
    default:
      return category;
  }
}

export function tStatusLabel(t, status) {
  switch (status) {
    case 0:
      return t('进行中');
    case 1:
      return t('等待回复');
    case 2:
      return t('已解决');
    case 3:
      return t('已关闭');
    default:
      return String(status);
  }
}

/**
 * 在 open / pending 状态下区分等待方。
 *   lastReplyRole: 0 user, 1 admin, 2 system
 *   isAdminView=true 时 "你" 指管理员；false 时 "你" 指用户。
 * 状态值不是 open/pending，就回退到 tStatusLabel。
 */
export function tDynamicStatusLabel(t, status, lastReplyRole, isAdminView) {
  if (status !== 0 && status !== 1) {
    return tStatusLabel(t, status);
  }
  if (lastReplyRole === 1) {
    // 管理员最后回复 → 等用户响应
    return isAdminView ? t('等待用户回复') : t('等待你回复');
  }
  // user 或 system 最后回复 → 等管理员处理
  return isAdminView ? t('等待你处理') : t('等待管理员回复');
}

export function tRole(t, role) {
  switch (role) {
    case 0:
      return t('用户');
    case 1:
      return t('管理员');
    case 2:
      return t('系统');
    default:
      return String(role);
  }
}

export function tPriorityLabel(t, p) {
  switch (p) {
    case 0:
      return t('低');
    case 1:
      return t('普通');
    case 2:
      return t('高');
    case 3:
      return t('紧急');
    default:
      return String(p);
  }
}

export function fmtTime(ts) {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
}

// 退款方式 / 退款原因 label。后端枚举值见 service/ticket/refund.go。
// 同样为了 i18next-cli 静态扫描器能采到 key，文案写成 t('字面量')。
export function tRefundMethod(t, m) {
  switch (m) {
    case 'platform':
      return t('平台充值');
    case 'offline':
      return t('线下充值');
    default:
      return m;
  }
}

export function tRefundReason(t, r) {
  switch (r) {
    case 'wrong_amount':
      return t('充错金额');
    case 'duplicate':
      return t('重复充值');
    case 'unused':
      return t('不再使用');
    case 'dissatisfied':
      return t('服务不满意');
    case 'other':
      return t('其他');
    default:
      return r;
  }
}

// 提供给建单页生成下拉项。集中在 constants 里避免 index.jsx 与 Detail.jsx 双份。
export const REFUND_METHODS = ['platform', 'offline'];
export const REFUND_REASONS = [
  'wrong_amount',
  'duplicate',
  'unused',
  'dissatisfied',
  'other',
];

// 工单分类白名单。与后端 model/ticket.go 的 ticketSupportCategories /
// ticketFeedbackCategories 完全对齐；列表筛选 / 建单页分类下拉共用。
// 新增分类时同步后端白名单 + tCategory 翻译。
export const TICKET_CATEGORIES_BY_TYPE = {
  support: [
    'model_invocation',
    'channel_issue',
    'billing',
    'account',
    'abuse',
    'refund',
    'other',
  ],
  feedback: ['feature', 'ux', 'docs', 'other'],
};

/**
 * 构造工单分类下拉项。type 为空时返回两类合并去重后的全集。
 * 用于列表页筛选区——筛选不一定先选 type。
 *   t：i18n hook
 *   type：'' | 'support' | 'feedback'
 *   overrides：可选 map（用户侧可传 setting.categories 覆盖默认列表）
 */
export function buildCategoryOptions(t, type, overrides) {
  const src = overrides || TICKET_CATEGORIES_BY_TYPE;
  let cats = [];
  if (type === 'support' || type === 'feedback') {
    cats = src[type] || [];
  } else {
    const seen = new Set();
    Object.values(src).forEach((arr) => {
      (arr || []).forEach((c) => seen.add(c));
    });
    cats = [...seen];
  }
  return cats.map((c) => ({ label: tCategory(t, c), value: c }));
}
