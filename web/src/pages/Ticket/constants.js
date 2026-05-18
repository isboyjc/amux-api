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
