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

import React from 'react';
import {
  Modal,
  Button,
  Input,
  Table,
  Tag,
  Typography,
  Switch,
  Banner,
  Tooltip,
} from '@douyinfe/semi-ui';
import { IconSearch, IconInfoCircle, IconCopy } from '@douyinfe/semi-icons';
import { MODEL_TABLE_PAGE_SIZE } from '../../../../constants';
import {
  WORKSPACE_COLOR,
  getWorkspaceLabel,
  inferWorkspaceFromModality,
} from '../../../../constants/workspaceTypes';
import {
  resolveTestProfile,
  UNTESTABLE_STT,
} from '../../../../constants/tokenTest';

const TokenTestModal = ({
  visible,
  record,
  models,
  modalityMap,
  loadingModels,
  results,
  testing,
  isBatchTesting,
  isStream,
  setIsStream,
  keyword,
  setKeyword,
  page,
  setPage,
  selectedKeys,
  setSelectedKeys,
  curlFallback,
  setCurlFallback,
  onClose,
  onTestModel,
  onCopyCurl,
  onBatchTest,
  onStopBatch,
  isMobile,
  t,
}) => {
  const filtered = models.filter((m) =>
    m.toLowerCase().includes(keyword.toLowerCase()),
  );

  const pageModels = (() => {
    const start = (page - 1) * MODEL_TABLE_PAGE_SIZE;
    return filtered.slice(start, start + MODEL_TABLE_PAGE_SIZE);
  })();

  // 流式只对 chat 类端点有意义；当前页没有任何 text/multimodal 模型时禁用开关。
  const streamApplicable = pageModels.some((m) => {
    const modality = modalityMap[m]?.modality || 'text';
    return Boolean(resolveTestProfile(m, modality).profile?.stream);
  });

  const renderModality = (model) => {
    const modality = modalityMap[model]?.modality || 'text';
    const workspace = inferWorkspaceFromModality(modality);
    return (
      <Tag color={WORKSPACE_COLOR[workspace] || 'grey'} shape='circle'>
        {getWorkspaceLabel(t, workspace)}
      </Tag>
    );
  };

  const renderResult = (model) => {
    const result = results[model];
    if (testing.has(model)) {
      return (
        <Tag color='blue' shape='circle'>
          {t('测试中')}
        </Tag>
      );
    }
    if (!result) {
      return (
        <Tag color='grey' shape='circle'>
          {t('未开始')}
        </Tag>
      );
    }

    if (result.untestable === UNTESTABLE_STT) {
      return (
        <Typography.Text type='tertiary' size='small'>
          {t('语音识别需上传音频文件，请到操练场测试')}
        </Typography.Text>
      );
    }

    if (result.success) {
      return (
        <div className='flex flex-col gap-1'>
          <div className='flex items-center gap-2 flex-wrap'>
            <Tag color='green' shape='circle'>
              {t('成功')}
            </Tag>
            <Typography.Text type='tertiary' size='small'>
              {t('耗时 ${time}s').replace('${time}', result.time.toFixed(2))}
            </Typography.Text>
            {result.summary && (
              <Typography.Text type='tertiary' size='small'>
                {result.summary}
              </Typography.Text>
            )}
          </div>
          {result.async && (
            <Typography.Text type='warning' size='small'>
              {t('视频生成为异步任务，此处仅验证提交成功，未等待生成完成')}
            </Typography.Text>
          )}
        </div>
      );
    }

    return (
      <div className='flex flex-col gap-1'>
        <div className='flex items-center gap-2 flex-wrap'>
          <Tag color='red' shape='circle'>
            {t('失败')}
          </Tag>
          {result.status && (
            <Typography.Text type='tertiary' size='small'>
              {/* HTTP 是协议名，不进翻译词表 */}
              {`HTTP ${result.status}`}
            </Typography.Text>
          )}
        </div>
        {result.rateLimited && (
          <Typography.Text type='warning' size='small'>
            {t('触发请求频率限制，不是令牌或模型的问题，请稍后重试')}
          </Typography.Text>
        )}
        {result.message && (
          <Typography.Text
            type='danger'
            size='small'
            className='break-all'
            style={{ maxWidth: '420px', fontSize: '12px' }}
          >
            {result.message}
          </Typography.Text>
        )}
      </div>
    );
  };

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'model',
      render: (text) => <Typography.Text strong>{text}</Typography.Text>,
    },
    {
      title: t('类型'),
      dataIndex: 'modality',
      render: (text, r) => renderModality(r.model),
    },
    {
      title: t('结果'),
      dataIndex: 'result',
      render: (text, r) => renderResult(r.model),
    },
    {
      title: '',
      dataIndex: 'operate',
      render: (text, r) => {
        const modality = modalityMap[r.model]?.modality || 'text';
        const { profile } = resolveTestProfile(r.model, modality);
        if (!profile) {
          return (
            <Tooltip
              content={t('语音识别需上传音频文件，请到操练场测试')}
              position='top'
            >
              <Button type='tertiary' size='small' disabled>
                {t('测试')}
              </Button>
            </Tooltip>
          );
        }
        return (
          <div className='flex items-center gap-1'>
            <Button
              type='tertiary'
              size='small'
              loading={testing.has(r.model)}
              disabled={isBatchTesting}
              onClick={() => onTestModel(r.model)}
            >
              {t('测试')}
            </Button>
            {/* 复制的命令与「测试」发出的请求完全一致（含当前流式开关状态），
                用户可直接在终端复现 */}
            <Tooltip content={t('复制等价的 cURL 命令')} position='top'>
              <Button
                type='tertiary'
                size='small'
                icon={<IconCopy />}
                aria-label={t('复制等价的 cURL 命令')}
                onClick={() => onCopyCurl(r.model)}
              >
                {/* cURL 是工具名，不进翻译词表 */}
                cURL
              </Button>
            </Tooltip>
          </div>
        );
      },
    },
  ];

  const dataSource = pageModels.map((model) => ({ model, key: model }));

  const handleBatch = () => {
    // 批量测试会把这一批模型全部真实调用一遍并计费，先让用户确认数量。
    const targets = selectedKeys.length > 0 ? selectedKeys : pageModels;
    Modal.confirm({
      title: t('确认批量测试？'),
      content: t(
        '将真实调用 ${count} 个模型并按实际用量计费，测试记录会出现在使用日志中。',
      ).replace('${count}', targets.length),
      onOk: () => onBatchTest(targets),
    });
  };

  return (
    <>
      <Modal
        title={
          record ? (
            <div className='flex items-center gap-2 flex-wrap'>
              <Typography.Text
                strong
                className='!text-[var(--semi-color-text-0)] !text-base'
              >
                {record.name} {t('的可用性测试')}
              </Typography.Text>
              <Typography.Text type='tertiary' size='small'>
                {t('共')} {models.length} {t('个模型')}
              </Typography.Text>
            </div>
          ) : null
        }
        visible={visible}
        onCancel={onClose}
        footer={
          <div className='flex justify-end gap-2'>
            {isBatchTesting ? (
              <Button type='danger' onClick={onStopBatch}>
                {t('停止测试')}
              </Button>
            ) : (
              <Button type='tertiary' onClick={onClose}>
                {t('关闭')}
              </Button>
            )}
            <Button
              onClick={handleBatch}
              loading={isBatchTesting}
              disabled={isBatchTesting || filtered.length === 0}
            >
              {selectedKeys.length > 0
                ? t('测试已选 ${count} 个').replace(
                    '${count}',
                    selectedKeys.length,
                  )
                : t('测试当前页 ${count} 个').replace(
                    '${count}',
                    pageModels.length,
                  )}
            </Button>
          </div>
        }
        maskClosable={!isBatchTesting}
        className='!rounded-lg'
        size={isMobile ? 'full-width' : 'large'}
      >
        <div className='flex flex-col gap-2'>
          <Banner
            type='warning'
            closeIcon={null}
            icon={<IconInfoCircle />}
            className='!rounded-lg'
            description={t(
              '测试会使用该令牌真实调用上游并产生费用（用量极小），同时会在使用日志中留下记录。分组由令牌自身配置决定，此处不可更改。',
            )}
          />

          <div className='flex flex-col sm:flex-row sm:items-center gap-2 w-full'>
            <Input
              placeholder={t('搜索模型...')}
              value={keyword}
              onChange={(v) => {
                setKeyword(v);
                setPage(1);
              }}
              className='!w-full sm:!flex-1'
              prefix={<IconSearch />}
              showClear
            />
            <div className='flex items-center justify-end gap-2 shrink-0'>
              <Typography.Text strong className='shrink-0'>
                {t('流式')}:
              </Typography.Text>
              <Switch
                checked={isStream}
                onChange={setIsStream}
                size='small'
                disabled={!streamApplicable || isBatchTesting}
                aria-label={t('流式')}
              />
            </div>
          </div>

          <Table
            columns={columns}
            dataSource={dataSource}
            loading={loadingModels}
            rowSelection={{
              selectedRowKeys: selectedKeys,
              onChange: (keys) => setSelectedKeys(keys || []),
            }}
            pagination={{
              currentPage: page,
              pageSize: MODEL_TABLE_PAGE_SIZE,
              total: filtered.length,
              showSizeChanger: false,
              onPageChange: (p) => setPage(p),
            }}
          />
        </div>
      </Modal>

      {/* 剪贴板不可用时（无权限 / 非 HTTPS 上下文）的兜底，让用户手动复制 */}
      <Modal
        title={t('请手动复制 cURL 命令')}
        visible={Boolean(curlFallback)}
        onCancel={() => setCurlFallback(null)}
        footer={
          <Button type='tertiary' onClick={() => setCurlFallback(null)}>
            {t('关闭')}
          </Button>
        }
        size={isMobile ? 'full-width' : 'large'}
        className='!rounded-lg'
      >
        <Typography.Paragraph
          copyable={{ content: curlFallback?.command || '' }}
          className='!mb-0'
        >
          <pre className='whitespace-pre-wrap break-all !text-xs !m-0'>
            {curlFallback?.command || ''}
          </pre>
        </Typography.Paragraph>
      </Modal>
    </>
  );
};

export default TokenTestModal;
