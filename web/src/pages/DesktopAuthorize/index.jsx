import React, { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Button,
  Typography,
  Space,
  Spin,
} from '@douyinfe/semi-ui';
import {
  IconTick,
  IconClose,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { API, showError } from '../../helpers';

const { Title, Text } = Typography;

export default function DesktopAuthorize() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const sessionId = searchParams.get('session_id');

  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [status, setStatus] = useState('checking'); // checking | valid | error | approved | rejected

  useEffect(() => {
    if (!sessionId) {
      setStatus('error');
      setLoading(false);
      return;
    }

    if (!localStorage.getItem('user')) {
      const returnUrl = `/desktop/authorize?session_id=${sessionId}`;
      navigate(`/login?callback=${encodeURIComponent(returnUrl)}`, { replace: true });
      return;
    }

    API.get(`/api/desktop/auth/info?session_id=${encodeURIComponent(sessionId)}`)
      .then((res) => {
        if (res.data.success) {
          setStatus('valid');
        } else {
          setStatus('error');
        }
      })
      .catch(() => {
        setStatus('error');
      })
      .finally(() => {
        setLoading(false);
      });
  }, [sessionId, navigate]);

  const handleApprove = async () => {
    setSubmitting(true);
    try {
      const res = await API.post('/api/desktop/auth/confirm', {
        session_id: sessionId,
        action: 'approve',
      });
      if (res.data.success) {
        setStatus('approved');
      } else {
        showError(res.data.message);
      }
    } catch {
      showError(t('操作失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleReject = async () => {
    setSubmitting(true);
    try {
      await API.post('/api/desktop/auth/confirm', {
        session_id: sessionId,
        action: 'reject',
      });
      setStatus('rejected');
    } catch {
      showError(t('操作失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const LogoIcon = () => (
    <div
      className='w-16 h-16 flex items-center justify-center mx-auto mb-4'
      style={{
        borderRadius: '16px',
        backgroundColor: '#fff',
        border: '1px solid var(--semi-color-border)',
      }}
    >
      <svg
        width='128'
        height='128'
        viewBox='0 0 128 128'
        fill='none'
        xmlns='http://www.w3.org/2000/svg'
        className='w-9 h-9'
      >
        <path
          d='M4 96 C4 96, 24 12, 64 12 C104 12, 124 96, 124 96 Q124 102, 118 102 C94 102, 92 64, 64 64 C36 64, 34 102, 10 102 Q4 102, 4 96 Z'
          fill='#000'
        />
      </svg>
    </div>
  );

  const renderContent = () => {
    if (loading) {
      return (
        <div className='flex justify-center items-center py-16'>
          <Spin size='large' />
        </div>
      );
    }

    if (status === 'error') {
      return (
        <Card className='!rounded-2xl w-full max-w-md'>
          <div className='text-center py-8'>
            <div
              className='w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-4'
              style={{ backgroundColor: 'var(--semi-color-danger-light-default)' }}
            >
              <IconClose size='extra-large' style={{ color: 'var(--semi-color-danger)' }} />
            </div>
            <Title heading={5} className='mb-2'>
              {t('授权链接无效')}
            </Title>
            <Text type='tertiary'>
              {t('授权链接无效或已过期，请返回 Desktop 重试')}
            </Text>
          </div>
        </Card>
      );
    }

    if (status === 'approved') {
      return (
        <Card className='!rounded-2xl w-full max-w-md'>
          <div className='text-center py-8'>
            <div
              className='w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-4'
              style={{ backgroundColor: 'var(--semi-color-success-light-default)' }}
            >
              <IconTick size='extra-large' style={{ color: 'var(--semi-color-success)' }} />
            </div>
            <Title heading={5} className='mb-2'>
              {t('授权成功')}
            </Title>
            <Text type='tertiary'>
              {t('你可以关闭此页面返回 Amux Desktop')}
            </Text>
          </div>
        </Card>
      );
    }

    if (status === 'rejected') {
      return (
        <Card className='!rounded-2xl w-full max-w-md'>
          <div className='text-center py-8'>
            <div className='w-16 h-16 rounded-full bg-semi-color-fill-0 flex items-center justify-center mx-auto mb-4'>
              <IconClose size='extra-large' style={{ color: 'var(--semi-color-tertiary)' }} />
            </div>
            <Title heading={5} className='mb-2'>
              {t('已取消授权')}
            </Title>
            <Text type='tertiary'>
              {t('你已拒绝此次授权请求')}
            </Text>
          </div>
        </Card>
      );
    }

    // status === 'valid'
    return (
      <Card className='!rounded-2xl w-full max-w-md'>
        <div className='py-4'>
          <div className='text-center mb-6'>
            <LogoIcon />
            <Title heading={4} className='mb-2'>
              {t('Amux Desktop 请求接入你的账号')}
            </Title>
            <Text type='tertiary' size='small'>
              {t('该操作将授予 Amux Desktop 你的系统访问令牌')}
            </Text>
          </div>

          <div
            className='rounded-xl p-4 mb-4'
            style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
          >
            <Text type='secondary' size='small' className='mb-3 block' style={{ fontWeight: 500 }}>
              {t('授权后 Amux Desktop 将可以：')}
            </Text>
            <Space vertical align='start' className='w-full'>
              <div className='flex items-start'>
                <IconTick size='small' style={{ color: 'var(--semi-color-success)' }} className='mr-2 mt-0.5 flex-shrink-0' />
                <Text size='small'>{t('获取你的账户基本信息')}</Text>
              </div>
              <div className='flex items-start'>
                <IconTick size='small' style={{ color: 'var(--semi-color-success)' }} className='mr-2 mt-0.5 flex-shrink-0' />
                <Text size='small'>{t('查看可用模型列表与分组信息')}</Text>
              </div>
              <div className='flex items-start'>
                <IconTick size='small' style={{ color: 'var(--semi-color-success)' }} className='mr-2 mt-0.5 flex-shrink-0' />
                <Text size='small'>{t('使用你的额度进行 API 调用')}</Text>
              </div>
            </Space>
          </div>

          <div
            className='rounded-xl p-4 mb-6 flex items-start gap-2'
            style={{
              backgroundColor: 'var(--semi-color-warning-light-default)',
            }}
          >
            <IconAlertTriangle size='small' style={{ color: 'var(--semi-color-warning)' }} className='flex-shrink-0 mt-0.5' />
            <Text size='small' type='secondary'>
              {t('系统访问令牌具有较高权限，请勿泄露给他人。如需撤销授权，可在个人设置中重新生成令牌以覆盖当前令牌。')}
            </Text>
          </div>

          <div className='flex gap-3'>
            <Button
              size='large'
              className='flex-1'
              onClick={handleReject}
              disabled={submitting}
            >
              {t('拒绝')}
            </Button>
            <Button
              type='primary'
              theme='solid'
              size='large'
              className='flex-1'
              onClick={handleApprove}
              loading={submitting}
            >
              {t('确认授权')}
            </Button>
          </div>
        </div>
      </Card>
    );
  };

  return (
    <div
      className='min-h-screen flex items-center justify-center p-4'
      style={{ backgroundColor: 'var(--semi-color-bg-0)' }}
    >
      {renderContent()}
    </div>
  );
}
