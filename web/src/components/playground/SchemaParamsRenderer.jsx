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
  Button,
  Input,
  InputNumber,
  Select,
  Slider,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { ImagePlus, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { showError } from '../../helpers';

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

// isImageInput 判断某个 property 是否是"图像输入槽"声明：
//   - 单图：{ type: 'string', format: 'image' }
//   - 多图：{ type: 'array', items: { type: 'string', format: 'image' } }
//
// 识别后这些字段会从"参数旋钮"组里移走，改由 SchemaInputsRenderer 渲染
// 为上传区。单一事实源：schema 里声明了 image 槽即代表模型支持参考图。
export const isImageInput = (def) => {
  if (!def || typeof def !== 'object') return false;
  if (def.format === 'image') return true;
  if (def.type === 'array' && def.items?.format === 'image') return true;
  return false;
};

// splitSchema 把一份 JSON Schema 按"控件类型"拆成两份：
//   - inputsSchema: 只含图像输入槽（SchemaInputsRenderer 用）
//   - paramsSchema: 其余所有标量旋钮（SchemaParamsRenderer 用）
//
// 返回的两份 schema 都保留原始结构（type/properties），properties 可能为空。
// 若原 schema 里没有图像槽，inputsSchema.properties 就是 {}，
// ImageWorkspace 会据此隐藏附件条，不影响现有模型。
export const splitSchema = (schema) => {
  const empty = { type: 'object', properties: {} };
  if (!schema || typeof schema !== 'object') {
    return { paramsSchema: empty, inputsSchema: empty };
  }
  const props = schema.properties || {};
  const params = {};
  const inputs = {};
  Object.entries(props).forEach(([k, def]) => {
    if (isImageInput(def)) inputs[k] = def;
    else params[k] = def;
  });
  return {
    paramsSchema: { ...schema, properties: params },
    inputsSchema: { ...schema, properties: inputs },
  };
};

// 按当前分组过滤 schema 的 properties。约定 schema 字段里可加扩展键
// `x-disabled-group-prefixes: ["special"]` 来声明「当用户选的分组前缀
// 命中其中任一项时，这个参数就不渲染、也不参与发送」。
//
// 设计上是「负向声明」——大多数参数官方/反代都通用，只有少数（如 n、
// background:transparent 这种反代不可靠的字段）需要显式标记。group key
// 用前缀匹配是因为我们只有两类约定：special* / premium*。
export const filterSchemaByGroup = (schema, group) => {
  if (!schema || typeof schema !== 'object') return schema;
  const props = schema.properties || {};
  if (!group || Object.keys(props).length === 0) return schema;

  const next = {};
  Object.entries(props).forEach(([key, def]) => {
    const disabled = def?.['x-disabled-group-prefixes'];
    if (
      Array.isArray(disabled) &&
      disabled.some(
        (p) => typeof p === 'string' && p && group.startsWith(p),
      )
    ) {
      return; // 当前分组命中黑名单前缀 → 跳过该字段
    }
    next[key] = def;
  });
  return { ...schema, properties: next };
};

// hasImageInputSlot 快速判断 schema 是否声明了至少一个图像槽，
// 供 ImageWorkspace 决定是否显示附件条 / "继续编辑"按钮。
export const hasImageInputSlot = (schema) => {
  const props = schema?.properties || {};
  return Object.values(props).some(isImageInput);
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

  // schema 扩展：enumLabels 把原始值映射成更友好的展示文案（如 0 → 「自动」），
  // 跨工具栏 / 右栏统一一份查表函数
  const enumLabelMap =
    def?.enumLabels && typeof def.enumLabels === 'object' ? def.enumLabels : null;
  const labelOf = (raw) =>
    enumLabelMap && Object.prototype.hasOwnProperty.call(enumLabelMap, String(raw))
      ? String(enumLabelMap[String(raw)])
      : String(raw);

  const renderControl = () => {
    if (hasEnum) {
      return (
        <Select
          value={value ?? def.default ?? def.enum[0]}
          onChange={onChange}
          size='small'
          style={{ width: '100%' }}
          optionList={def.enum.map((e) => ({ label: labelOf(e), value: e }))}
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

// ==== 图像输入槽渲染器 ====
//
// 设计对齐 SchemaParamsRenderer：一份 schema → 一组受控控件。区别是
// 这里渲染的是"资源输入"（File / File[]），不是标量参数。
//
// values 形状：{ [propertyKey]: File | File[] | null }
//   - 单图槽（format:image）：File | null
//   - 多图槽（type:array + items.format:image）：File[]
//
// 上传后文件以 File 对象常驻 React state，发送请求时由 useImageGeneration
// 打包进 FormData。不在这里做 base64 编码，避免大图占内存。

const DEFAULT_ACCEPT = 'image/*';
const FALLBACK_MAX_SIZE = 10 * 1024 * 1024; // 10MB — schema 没声明时的兜底

// 容错读出 File 的缩略图 URL。返回 cleanup 函数以便卸载时 revoke。
const useObjectUrls = (files) => {
  const urlsRef = React.useRef(new Map());
  const [, force] = React.useState(0);

  React.useEffect(() => {
    const seen = new Set();
    (files || []).forEach((f) => {
      if (!(f instanceof File)) return;
      seen.add(f);
      if (!urlsRef.current.has(f)) {
        urlsRef.current.set(f, URL.createObjectURL(f));
        force((x) => x + 1);
      }
    });
    // revoke 掉本轮不再出现的文件对应 URL
    for (const [f, url] of urlsRef.current.entries()) {
      if (!seen.has(f)) {
        URL.revokeObjectURL(url);
        urlsRef.current.delete(f);
      }
    }
  }, [files]);

  React.useEffect(() => {
    return () => {
      for (const url of urlsRef.current.values()) URL.revokeObjectURL(url);
      urlsRef.current.clear();
    };
  }, []);

  return (file) => urlsRef.current.get(file);
};

// Thumbnail：单张缩略图 + 右上角删除。高度固定，宽度按原比例。
const Thumbnail = ({ url, label, onRemove, t }) => (
  <div
    className='relative rounded-md overflow-hidden flex-shrink-0 group'
    style={{
      width: 64,
      height: 64,
      border: '1px solid var(--semi-color-border)',
      backgroundColor: 'var(--semi-color-fill-0)',
    }}
    title={label}
  >
    {url ? (
      <img
        src={url}
        alt={label || ''}
        style={{
          width: '100%',
          height: '100%',
          objectFit: 'cover',
          display: 'block',
        }}
      />
    ) : null}
    <button
      type='button'
      onClick={(e) => {
        e.stopPropagation();
        onRemove?.();
      }}
      aria-label={t('移除')}
      className='absolute opacity-0 group-hover:opacity-100 transition-opacity'
      style={{
        top: 2,
        right: 2,
        width: 18,
        height: 18,
        borderRadius: '50%',
        border: 'none',
        background: 'rgba(0,0,0,0.6)',
        color: 'white',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        padding: 0,
      }}
    >
      <X size={12} />
    </button>
  </div>
);

const ImageSlot = ({ fieldKey, def, value, onChange, disabled, t }) => {
  const inputRef = React.useRef(null);
  const isArray = def?.type === 'array';
  const maxItems = isArray ? def?.maxItems ?? 4 : 1;
  const maxSize = def?.maxSize ?? FALLBACK_MAX_SIZE;
  const accept = def?.accept || DEFAULT_ACCEPT;
  const label = def?.title || fieldKey;
  const description = def?.description;

  const currentList = React.useMemo(() => {
    if (!value) return [];
    if (Array.isArray(value)) return value.filter((x) => x instanceof File);
    return value instanceof File ? [value] : [];
  }, [value]);

  const getUrlFor = useObjectUrls(currentList);

  const validate = (file) => {
    if (!(file instanceof File)) return false;
    if (maxSize && file.size > maxSize) {
      showError(
        t('{{name}} 超过 {{mb}}MB 上限', {
          name: file.name,
          mb: (maxSize / 1024 / 1024).toFixed(1),
        }),
      );
      return false;
    }
    if (accept && accept !== '*/*' && accept !== 'image/*') {
      // 简单 MIME 白名单校验：accept 支持 "image/png,image/jpeg" 这样的列表
      const allowed = accept.split(',').map((s) => s.trim());
      const ok = allowed.some((a) => {
        if (a.endsWith('/*')) return file.type.startsWith(a.slice(0, -1));
        return file.type === a;
      });
      if (!ok) {
        showError(t('{{name}} 类型不支持', { name: file.name }));
        return false;
      }
    } else if (accept === 'image/*' && !file.type.startsWith('image/')) {
      showError(t('{{name}} 不是图像', { name: file.name }));
      return false;
    }
    return true;
  };

  const addFiles = (files) => {
    const valid = Array.from(files || []).filter(validate);
    if (valid.length === 0) return;
    if (!isArray) {
      onChange?.(valid[0]);
      return;
    }
    const next = [...currentList, ...valid].slice(0, maxItems);
    onChange?.(next);
  };

  const removeAt = (idx) => {
    if (!isArray) {
      onChange?.(null);
      return;
    }
    const next = currentList.filter((_, i) => i !== idx);
    onChange?.(next.length === 0 ? [] : next);
  };

  const full = currentList.length >= maxItems;
  const canAdd = !disabled && !full;

  const handleFilePick = (e) => {
    addFiles(e.target.files);
    e.target.value = ''; // 允许反复选同一个文件
  };

  const handleDrop = (e) => {
    e.preventDefault();
    if (!canAdd) return;
    if (e.dataTransfer?.files) addFiles(e.dataTransfer.files);
  };

  return (
    <div className='mb-3'>
      <div className='flex items-baseline justify-between mb-1.5'>
        <Typography.Text strong className='text-sm'>
          {t(label)}
        </Typography.Text>
        {isArray && (
          <Typography.Text type='tertiary' className='text-xs'>
            {currentList.length}/{maxItems}
          </Typography.Text>
        )}
      </div>

      <div className='flex items-center gap-2 flex-wrap'>
        {currentList.map((f, i) => (
          <Thumbnail
            key={`${f.name}-${i}-${f.size}`}
            url={getUrlFor(f)}
            label={f.name}
            onRemove={() => removeAt(i)}
            t={t}
          />
        ))}
        {canAdd && (
          <div
            role='button'
            tabIndex={0}
            onClick={() => inputRef.current?.click()}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                inputRef.current?.click();
              }
            }}
            onDragOver={(e) => e.preventDefault()}
            onDrop={handleDrop}
            className='rounded-md flex items-center justify-center cursor-pointer transition-colors'
            style={{
              width: 64,
              height: 64,
              border: '1px dashed var(--semi-color-border)',
              backgroundColor: 'var(--semi-color-fill-0)',
              color: 'var(--semi-color-text-2)',
            }}
            title={t('点击或拖拽上传')}
          >
            <ImagePlus size={20} />
          </div>
        )}
        <input
          ref={inputRef}
          type='file'
          accept={accept}
          multiple={isArray}
          disabled={disabled}
          onChange={handleFilePick}
          style={{ display: 'none' }}
        />
      </div>

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
 * SchemaInputsRenderer - 渲染 schema 中所有图像输入槽。
 *
 * @param schema   只含图像槽的 schema（由 splitSchema 拆出）
 * @param values   { [key]: File | File[] | null }
 * @param onChange (nextValues) => void
 * @param disabled 全局禁用
 * @param compact  紧凑展示（忽略描述文字）
 */
export const SchemaInputsRenderer = ({
  schema,
  values = {},
  onChange,
  disabled,
}) => {
  const { t } = useTranslation();
  const props = schema?.properties || {};
  const entries = Object.entries(props).filter(([, def]) => isImageInput(def));
  if (entries.length === 0) return null;
  return (
    <div>
      {entries.map(([k, def]) => (
        <ImageSlot
          key={k}
          fieldKey={k}
          def={def}
          value={values[k]}
          disabled={disabled}
          t={t}
          onChange={(v) => onChange?.({ ...values, [k]: v })}
        />
      ))}
    </div>
  );
};

// hasAnyImageValue 判断 values 里是否至少一个图像槽被填了。
// useImageGeneration 据此决定走 JSON generations 还是 multipart edits。
export const hasAnyImageValue = (values) => {
  if (!values || typeof values !== 'object') return false;
  return Object.values(values).some((v) => {
    if (v instanceof File) return true;
    if (Array.isArray(v)) return v.some((x) => x instanceof File);
    return false;
  });
};
