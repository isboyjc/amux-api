import React, { useEffect, useState, useCallback } from 'react';
import { Button, Input, Switch, Typography } from '@douyinfe/semi-ui';
import { Headset, Plus, Trash2, Link2 } from 'lucide-react';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const MAX_ITEMS = 3;

const SettingsSupport = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [email, setEmail] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [saving, setSaving] = useState(false);

  const updateOption = async (key, value) => {
    const res = await API.put('/api/option/', { key, value });
    if (!res.data.success) {
      showError(res.data.message);
      return false;
    }
    return true;
  };

  useEffect(() => {
    const supportStr =
      options['console_setting.support'] ?? options.Support;
    if (supportStr) {
      try {
        const data = JSON.parse(supportStr);
        setItems(data.items || []);
        setEmail(data.email || '');
      } catch {
        // ignore
      }
    }
  }, [options['console_setting.support'], options.Support]);

  useEffect(() => {
    const enabledStr = options['console_setting.support_enabled'];
    if (enabledStr !== undefined && enabledStr !== '') {
      setEnabled(enabledStr === 'true' || enabledStr === true);
    }
  }, [options['console_setting.support_enabled']]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const data = { items, email };
      const ok = await updateOption(
        'console_setting.support',
        JSON.stringify(data),
      );
      if (ok) {
        showSuccess(t('保存成功'));
        refresh();
      }
    } finally {
      setSaving(false);
    }
  }, [items, email, refresh, t]);

  const handleToggleEnabled = useCallback(
    async (val) => {
      setEnabled(val);
      await updateOption(
        'console_setting.support_enabled',
        String(val),
      );
      refresh();
    },
    [refresh],
  );

  const updateItem = (index, field, value) => {
    setItems((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value };
      return next;
    });
  };

  const addItem = () => {
    if (items.length >= MAX_ITEMS) return;
    setItems((prev) => [...prev, { url: '', label: '' }]);
  };

  const removeItem = (index) => {
    setItems((prev) => prev.filter((_, i) => i !== index));
  };

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between'>
        <div className='flex items-center gap-2'>
          <Headset size={18} />
          <Title heading={5} className='!mb-0'>
            {t('关于 Amux 社区')}
          </Title>
        </div>
        <Switch
          checked={enabled}
          onChange={handleToggleEnabled}
          size='small'
        />
      </div>

      <Text type='tertiary' size='small'>
        {t('配置 Amux 社区链接，最多3个，图片链接自动渲染图片，其他链接展示为浏览器打开入口')}
      </Text>

      {/* 链接配置项 */}
      <div className='space-y-3'>
        {items.map((item, index) => (
          <div
            key={index}
            className='flex items-start gap-3 p-3 rounded-xl bg-semi-color-fill-0'
          >
            <div className='w-10 h-10 rounded-lg bg-semi-color-bg-2 flex items-center justify-center flex-shrink-0'>
              <Link2 size={18} className='text-semi-color-text-3' />
            </div>
            <div className='flex-1 space-y-2'>
              <Input
                value={item.url || item.qrcode || ''}
                onChange={(v) => updateItem(index, 'url', v)}
                placeholder={t('链接地址（图片 URL 或网页 URL）')}
                size='small'
              />
              <Input
                value={item.label}
                onChange={(v) => updateItem(index, 'label', v)}
                placeholder={t('底部文案，如：微信交流群')}
                size='small'
              />
            </div>
            <Button
              icon={<Trash2 size={14} />}
              type='danger'
              theme='borderless'
              size='small'
              onClick={() => removeItem(index)}
            />
          </div>
        ))}

        {items.length < MAX_ITEMS && (
          <Button
            icon={<Plus size={14} />}
            theme='borderless'
            type='tertiary'
            size='small'
            onClick={addItem}
          >
            {t('添加链接')}
          </Button>
        )}
      </div>

      {/* 邮箱配置 */}
      <div>
        <Text size='small' className='mb-1 block'>
          {t('支持邮箱')}
        </Text>
        <Input
          value={email}
          onChange={setEmail}
          placeholder='support@example.com'
          size='small'
        />
      </div>

      <Button
        theme='solid'
        type='primary'
        size='small'
        loading={saving}
        onClick={handleSave}
      >
        {t('保存')}
      </Button>
    </div>
  );
};

export default SettingsSupport;
