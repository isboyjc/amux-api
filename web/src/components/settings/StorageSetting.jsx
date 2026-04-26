/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Form,
  List,
  Row,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconTick,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const { Text } = Typography;

// 后端 storage.* 配置键，与 setting/system_setting/storage.go 的 json
// tag 一一对应。任何一边新增字段都得双向同步。
const STORAGE_KEYS = {
  enabled: 'storage.enabled',
  provider: 'storage.provider',
  r2AccountId: 'storage.r2_account_id',
  r2AccessKeyId: 'storage.r2_access_key_id',
  r2Secret: 'storage.r2_secret', // 后端按 "secret" 后缀过滤，GET 不回显
  r2Bucket: 'storage.r2_bucket',
  r2Endpoint: 'storage.r2_endpoint',
  r2Region: 'storage.r2_region',
  r2PublicBaseUrl: 'storage.r2_public_base_url',
  imageTransformEnabled: 'storage.image_transform_enabled',
};

// 当前支持的 provider 选项；后续接入新提供方时直接在这里追加。
const PROVIDER_OPTIONS = [
  { label: 'Cloudflare R2', value: 'r2' },
];

// Semi Form 的 field 含 "." 会被当成嵌套路径（formValues.storage.enabled），
// 必须用 ['...'] 转义才能保留扁平 key。这是 Semi 通用约定，SystemSetting.jsx
// 里所有 *.enabled / *.client_id 也都是这么写的——不转义会导致 setValues
// 写不进、onValueChange 读不到，整个表单状态废掉、提交时 buildChangedOptions
// 完全不发请求。**只在 JSX 的 field 属性上用**：state / setValues / 提交体仍
// 用未转义的扁平 key（"storage.enabled"），Semi 内部会自动映射两边。
const ff = (key) => `['${key}']`;

const defaultInputs = () => ({
  [STORAGE_KEYS.enabled]: false,
  [STORAGE_KEYS.provider]: 'r2',
  [STORAGE_KEYS.r2AccountId]: '',
  [STORAGE_KEYS.r2AccessKeyId]: '',
  [STORAGE_KEYS.r2Secret]: '',
  [STORAGE_KEYS.r2Bucket]: '',
  [STORAGE_KEYS.r2Endpoint]: '',
  [STORAGE_KEYS.r2Region]: 'auto',
  [STORAGE_KEYS.r2PublicBaseUrl]: '',
  [STORAGE_KEYS.imageTransformEnabled]: false,
});

const StorageSetting = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState(defaultInputs);
  const [originInputs, setOriginInputs] = useState({});
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState(null); // { success, steps, bucket, test_key, test_public_url } | null
  const formApiRef = useRef(null);

  // 后端 step.name 是稳定枚举（config / presign / put / ...）；这里映射成
  // 本地化文案。**每个 t() 必须是字面量字符串**——i18next-cli extract 是
  // 静态扫描，t(变量) 抓不到 key、其它语种就没条目。
  const stepLabels = useMemo(
    () => ({
      config: t('配置完整性'),
      presign: t('预签名 URL 生成'),
      put: t('上传到 R2 (PUT)'),
      public_get: t('公网 URL 回读校验'),
      cleanup: t('清理测试对象'),
      request: t('发起测试请求'),
    }),
    [t],
  );

  // 拉一次现有 options。后端 GetOptions 会过滤掉 *secret 后缀字段，所以
  // r2_secret 永远是空——前端把它显示成「敏感信息不会发送到前端显示」，
  // 用户填了非空值就当作"覆盖"提交。
  const loadOptions = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const next = defaultInputs();
      // 哪些 key 是 bool 字段——load/save 时按 'true'/'false' 字符串与
      // 真布尔互转。新增 bool 字段记得加进来。
      const boolKeys = new Set([
        STORAGE_KEYS.enabled,
        STORAGE_KEYS.imageTransformEnabled,
      ]);
      data.forEach((item) => {
        if (!item || !Object.values(STORAGE_KEYS).includes(item.key)) return;
        // option 表里所有值都按字符串存；bool 字段需要在前端反序列化，
        // 否则 Switch 会拿到字符串"false"——它是 truthy，开关会反向显示
        if (boolKeys.has(item.key)) {
          next[item.key] = item.value === 'true' || item.value === true;
        } else {
          next[item.key] = item.value ?? '';
        }
      });
      // provider 取不到就强制 r2，避免下拉显示空
      if (!next[STORAGE_KEYS.provider]) {
        next[STORAGE_KEYS.provider] = 'r2';
      }
      // region 取不到就给 auto
      if (!next[STORAGE_KEYS.r2Region]) {
        next[STORAGE_KEYS.r2Region] = 'auto';
      }
      setInputs(next);
      setOriginInputs(next);
      if (formApiRef.current) {
        formApiRef.current.setValues(next);
      }
    } catch (err) {
      showError(err?.message || t('加载存储配置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadOptions();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  // 只把"真正改过"的字段提交：避免把空 secret 误覆盖成空字符串，把
  // 已配好的 R2 凭证抹掉。
  //
  // r2_secret 特殊处理：UI 永远显示为空（后端也不返回），用户输入了任何
  // 非空值才算改动并 PUT；空值 = 保持现状不动。
  const buildChangedOptions = () => {
    const changed = [];
    Object.values(STORAGE_KEYS).forEach((key) => {
      const cur = inputs[key];
      const orig = originInputs[key];
      if (key === STORAGE_KEYS.r2Secret) {
        // 只在用户填入非空 secret 时才提交
        const v = (cur || '').trim();
        if (v !== '') {
          changed.push({ key, value: v });
        }
        return;
      }
      if (
        key === STORAGE_KEYS.enabled ||
        key === STORAGE_KEYS.imageTransformEnabled
      ) {
        // bool → 'true'/'false' 字符串：option 表统一字符串存储；
        // 后端 LoadFromDB 时再反序列化回 bool
        const a = cur ? 'true' : 'false';
        const b = orig ? 'true' : 'false';
        if (a !== b) {
          changed.push({ key, value: a });
        }
        return;
      }
      // 其它字段：值变了就提交（包括清空场景）
      const a = (cur ?? '').toString();
      const b = (orig ?? '').toString();
      if (a !== b) {
        changed.push({ key, value: a });
      }
    });
    return changed;
  };

  // 测试连接：服务端拿当前 admin 面板里"已保存"的配置做端到端测试。
  // 注意：尚未保存的修改不会生效——必须先点保存，再测试。
  const runTest = async () => {
    setTestResult(null);
    setTesting(true);
    try {
      const res = await API.post('/api/upload/test', {});
      const { success, message, data } = res.data;
      if (!success) {
        // 后端业务级失败（凭证不全等）；data.steps 也会带，照样展示
        setTestResult(data || { success: false, steps: [] });
        if (message) showError(message);
        return;
      }
      setTestResult(data);
      if (data?.success) {
        showSuccess(t('对象存储连通正常'));
      } else {
        showError(t('对象存储测试失败，请查看下方步骤详情'));
      }
    } catch (err) {
      showError(err?.message || t('测试请求失败'));
      setTestResult({
        success: false,
        steps: [
          {
            name: 'request',
            ok: false,
            message: err?.message || t('测试请求失败'),
          },
        ],
      });
    } finally {
      setTesting(false);
    }
  };

  const submitAll = async () => {
    const options = buildChangedOptions();
    if (options.length === 0) {
      showSuccess(t('没有需要保存的更改'));
      return;
    }
    setLoading(true);
    try {
      const results = await Promise.all(
        options.map((opt) =>
          API.put('/api/option/', {
            key: opt.key,
            value: opt.value,
          }),
        ),
      );
      const failures = results.filter((res) => !res.data.success);
      if (failures.length > 0) {
        failures.forEach((f) => showError(f.data.message));
        return;
      }
      showSuccess(t('存储配置已更新'));
      // 重新拉一次，确保 originInputs 同步到最新；secret 字段拉回还是空
      await loadOptions();
    } catch (err) {
      showError(err?.message || t('更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const provider = inputs[STORAGE_KEYS.provider] || 'r2';

  return (
    <Spin spinning={loading} size='large'>
      <Form
        getFormApi={(api) => (formApiRef.current = api)}
        onValueChange={handleFormChange}
        initValues={inputs}
        labelPosition='top'
      >
        <Row gutter={16}>
          <Col xs={24}>
            <Card>
              <Form.Section text={t('对象存储设置')}>
                <Banner
                  type='info'
                  description={t(
                    '配置后用于操练场视频/图片/音频参考素材上传以及未来通用文件上传。即使凭证填齐，"启用"开关关闭时上传接口仍会返回 503——这样可以先填配置、点"测试连接"验证、再正式启用。配置变更立即生效，无需重启服务。',
                  )}
                  style={{ marginBottom: 16 }}
                />

                <Row gutter={16}>
                  <Col xs={24} sm={12}>
                    <Form.Switch
                      field={ff(STORAGE_KEYS.enabled)}
                      label={t('启用对象存储')}
                      extraText={t(
                        '关闭后上传接口直接返回 503；管理员"测试连接"按钮不受此开关影响',
                      )}
                    />
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Select
                      field={ff(STORAGE_KEYS.provider)}
                      label={t('存储提供方')}
                      optionList={PROVIDER_OPTIONS}
                      placeholder='r2'
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>

                {provider === 'r2' && (
                  <>
                    <Banner
                      type='warning'
                      description={t(
                        '在 Cloudflare 控制台 → R2 → "Manage R2 API Tokens" 创建权限为 Object Read & Write、并指定到目标桶的 API Token，把 Access Key ID / Secret Access Key 填到下面。R2 桶默认私有，必须在桶设置里启用 r2.dev 公开访问，或绑定自定义域名，并把对应公网地址填入"公网访问基地址"。',
                      )}
                      style={{ marginBottom: 16, marginTop: 16 }}
                    />

                    <Row gutter={16}>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2AccountId)}
                          label={t('Cloudflare 账号 ID')}
                          placeholder={t('如：125fa9ed84cf728a31bd360e52477e81')}
                          extraText={t(
                            '留空则需要单独填写完整的 S3 endpoint',
                          )}
                        />
                      </Col>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2Bucket)}
                          label={t('桶名')}
                          placeholder='amux'
                        />
                      </Col>
                    </Row>

                    <Row gutter={16}>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2AccessKeyId)}
                          label={t('Access Key ID')}
                          placeholder={t('R2 API Token 的 Access Key ID')}
                        />
                      </Col>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2Secret)}
                          label={t('Secret Access Key')}
                          type='password'
                          placeholder={t(
                            '敏感信息不会回显；留空则保持现有值不变',
                          )}
                        />
                      </Col>
                    </Row>

                    <Row gutter={16}>
                      <Col xs={24}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2PublicBaseUrl)}
                          label={t('公网访问基地址')}
                          placeholder={t(
                            '如：https://pub-xxxx.r2.dev 或绑定的自定义域名 https://files.your-domain.com（末尾不带 /）',
                          )}
                          extraText={t(
                            '前端拼接对象 URL 用。R2 桶私有时浏览器访问 404，必须配置此项',
                          )}
                        />
                      </Col>
                    </Row>

                    <Row gutter={16}>
                      <Col xs={24}>
                        <Form.Switch
                          field={ff(STORAGE_KEYS.imageTransformEnabled)}
                          label={t('启用 Cloudflare 图片优化')}
                          extraText={t(
                            '开启后，UI 缩略图自动走 cdn-cgi/image 生成 WebP/AVIF 缩略版本，省客户端带宽；发给上游模型的图片永远是原图，不受影响。仅在「公网访问基地址」是 Cloudflare 代理（橙云）的自定义域名 + Pro 及以上计划时有效，r2.dev 子域不支持。',
                          )}
                        />
                      </Col>
                    </Row>

                    <Row gutter={16}>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2Endpoint)}
                          label={t('S3 Endpoint（可选）')}
                          placeholder={t(
                            '留空时按账号 ID 派生为 https://${account_id}.r2.cloudflarestorage.com',
                          )}
                        />
                      </Col>
                      <Col xs={24} sm={12}>
                        <Form.Input
                          field={ff(STORAGE_KEYS.r2Region)}
                          label={t('Region')}
                          placeholder='auto'
                          extraText={t(
                            'R2 不分 region，固定写 auto；接其它 S3 兼容服务时按需修改',
                          )}
                        />
                      </Col>
                    </Row>
                  </>
                )}

                <div style={{ marginTop: 16 }}>
                  <Space>
                    <Button onClick={submitAll} type='primary'>
                      {t('保存存储配置')}
                    </Button>
                    <Button onClick={runTest} loading={testing}>
                      {t('测试连接')}
                    </Button>
                    <Text type='tertiary' size='small'>
                      {t(
                        '测试用的是已保存的配置；改完先点保存再测试',
                      )}
                    </Text>
                  </Space>
                </div>

                {testResult && (
                  <div style={{ marginTop: 16 }}>
                    <Banner
                      type={testResult.success ? 'success' : 'danger'}
                      icon={
                        testResult.success ? (
                          <IconTick />
                        ) : (
                          <IconAlertTriangle />
                        )
                      }
                      title={
                        testResult.success
                          ? t('对象存储连通正常')
                          : t('对象存储测试失败')
                      }
                      description={
                        testResult.success
                          ? t(
                              '服务器侧 PUT + 公网 GET 都通过。注意：浏览器直传 / 跨域回读还需要桶 CORS 放行前端域名（PUT + GET + Content-Type），CORS 错误这里测不到。',
                            )
                          : t(
                              '请按下方步骤详情逐项排查；任意一步失败都会终止后续测试。',
                            )
                      }
                      style={{ marginBottom: 12 }}
                    />
                    <List
                      size='small'
                      dataSource={testResult.steps || []}
                      renderItem={(step) => (
                        <List.Item
                          main={
                            <Space>
                              {step.ok ? (
                                <Tag
                                  color='green'
                                  prefixIcon={<IconTick />}
                                >
                                  {t('通过')}
                                </Tag>
                              ) : (
                                <Tag
                                  color='red'
                                  prefixIcon={<IconClose />}
                                >
                                  {t('失败')}
                                </Tag>
                              )}
                              <Text strong>
                                {stepLabels[step.name] || step.name}
                              </Text>
                              {step.message && (
                                <Text type='tertiary'>
                                  {step.message}
                                </Text>
                              )}
                            </Space>
                          }
                        />
                      )}
                    />
                    {testResult.test_public_url && (
                      <div style={{ marginTop: 8 }}>
                        <Text type='tertiary' size='small'>
                          {t('测试对象 URL')}:{' '}
                        </Text>
                        <Text
                          code
                          copyable
                          size='small'
                          style={{ wordBreak: 'break-all' }}
                        >
                          {testResult.test_public_url}
                        </Text>
                      </div>
                    )}
                  </div>
                )}
              </Form.Section>
            </Card>
          </Col>
        </Row>
      </Form>
    </Spin>
  );
};

export default StorageSetting;
