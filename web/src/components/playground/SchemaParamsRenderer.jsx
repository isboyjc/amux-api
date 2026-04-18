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
  Input,
  InputNumber,
  Select,
  Slider,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

// 解析 JSON Schema（字符串或对象）→ object
export const parseSchema = (raw) => {
  if (!raw) return null;
  if (typeof raw === 'object') return raw;
  try {
    const s = JSON.parse(raw);
    if (s && typeof s === 'object') return s;
  } catch {}
  return null;
};

// 从 schema 计算默认 values
export const defaultsOf = (schema) => {
  const props = schema?.properties || {};
  const out = {};
  Object.entries(props).forEach(([k, def]) => {
    if (def?.default !== undefined) out[k] = def.default;
    else if (Array.isArray(def?.enum) && def.enum.length > 0)
      out[k] = def.enum[0];
  });
  return out;
};

// 单个 schema property 渲染成对应控件
const SchemaField = ({
  fieldKey,
  def,
  value,
  onChange,
  disabled,
  compact,
  t,
}) => {
  const label = def?.title || fieldKey;
  const description = def?.description;
  const hasEnum = Array.isArray(def?.enum) && def.enum.length > 0;

  // 数字 + 有边界 → Slider
  const isBoundedNumber =
    (def?.type === 'number' || def?.type === 'integer') &&
    typeof def?.minimum === 'number' &&
    typeof def?.maximum === 'number';

  const renderControl = () => {
    if (hasEnum) {
      return (
        <Select
          value={value ?? def.default ?? def.enum[0]}
          onChange={onChange}
          size='small'
          style={{ width: '100%' }}
          optionList={def.enum.map((e) => ({ label: String(e), value: e }))}
          disabled={disabled}
          position={compact ? 'topLeft' : 'bottomLeft'}
        />
      );
    }

    if (def?.type === 'boolean') {
      return (
        <Switch
          checked={!!(value ?? def.default)}
          onChange={onChange}
          disabled={disabled}
        />
      );
    }

    if (isBoundedNumber) {
      const step = def.type === 'integer' ? 1 : def.step || 0.1;
      return (
        <div className='flex items-center gap-2'>
          <Slider
            value={value ?? def.default ?? def.minimum}
            onChange={onChange}
            min={def.minimum}
            max={def.maximum}
            step={step}
            style={{ flex: 1 }}
            disabled={disabled}
          />
          <InputNumber
            value={value ?? def.default ?? def.minimum}
            min={def.minimum}
            max={def.maximum}
            step={step}
            precision={def.type === 'integer' ? 0 : undefined}
            onNumberChange={onChange}
            size='small'
            style={{ width: 80 }}
            hideButtons
            disabled={disabled}
          />
        </div>
      );
    }

    if (def?.type === 'integer' || def?.type === 'number') {
      return (
        <InputNumber
          value={value ?? def.default}
          min={def.minimum}
          max={def.maximum}
          step={def.type === 'integer' ? 1 : def.step || 0.1}
          precision={def.type === 'integer' ? 0 : undefined}
          onNumberChange={onChange}
          size='small'
          style={{ width: '100%' }}
          hideButtons
          disabled={disabled}
        />
      );
    }

    return (
      <Input
        value={value ?? ''}
        onChange={onChange}
        size='small'
        disabled={disabled}
      />
    );
  };

  if (compact) {
    // 紧凑横排：标签 + 控件（供需要极节省垂直空间的地方使用）
    return (
      <div className='flex items-center gap-2'>
        <Typography.Text
          type='tertiary'
          className='text-xs flex-shrink-0'
          style={{ minWidth: 56 }}
        >
          {t(label)}
        </Typography.Text>
        <div className='flex-1 min-w-0'>{renderControl()}</div>
      </div>
    );
  }

  // 标准模式：标签在上 + 控件在下 + 可选描述
  return (
    <div className='mb-4'>
      <div className='flex items-baseline justify-between mb-1.5'>
        <Typography.Text strong className='text-sm'>
          {t(label)}
        </Typography.Text>
        {hasEnum || def?.type === 'boolean' ? null : (
          <Typography.Text type='tertiary' className='text-xs'>
            {value !== undefined && value !== null && value !== ''
              ? String(value)
              : ''}
          </Typography.Text>
        )}
      </div>
      {renderControl()}
      {description && (
        <Typography.Text
          type='tertiary'
          className='text-xs block mt-1'
          style={{ lineHeight: 1.4 }}
        >
          {t(description)}
        </Typography.Text>
      )}
    </div>
  );
};

/**
 * SchemaParamsRenderer - 根据 JSON Schema 渲染一整组参数控件。
 * @param schema  JSON Schema（object，含 properties）
 * @param values  当前值 { [fieldKey]: value }
 * @param onChange(nextValues)
 * @param disabled
 * @param compact 紧凑模式（标签同行）
 */
const SchemaParamsRenderer = ({
  schema,
  values = {},
  onChange,
  disabled,
  compact = false,
}) => {
  const { t } = useTranslation();
  const props = schema?.properties || {};
  const entries = Object.entries(props);
  if (entries.length === 0) {
    return (
      <Typography.Text type='tertiary' className='text-sm'>
        {t('该模型未声明任何参数')}
      </Typography.Text>
    );
  }
  return (
    <div className={compact ? 'space-y-2' : ''}>
      {entries.map(([k, def]) => (
        <SchemaField
          key={k}
          fieldKey={k}
          def={def}
          value={values[k]}
          disabled={disabled}
          compact={compact}
          t={t}
          onChange={(v) => onChange?.({ ...values, [k]: v })}
        />
      ))}
    </div>
  );
};

export default SchemaParamsRenderer;
