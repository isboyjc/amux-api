import React, { useState, useCallback, useRef, useMemo } from 'react';
import ReactDOM from 'react-dom';

const HEALTH_COLORS = {
  noData: 'var(--semi-color-fill-1)',
  healthy: '#22C55E',
  lightGreen: '#86EFAC',
  yellow: '#EAB308',
  orange: '#F59E0B',
  deepOrange: '#EA580C',
  red: '#EF4444',
};

const getHealthColor = (successCount, errorCount) => {
  const total = successCount + errorCount;
  if (total === 0) return HEALTH_COLORS.noData;
  if (total < 3 && errorCount > 0) return HEALTH_COLORS.orange;
  const errorRate = errorCount / total;
  if (errorRate === 0) return HEALTH_COLORS.healthy;
  if (errorRate < 0.02) return HEALTH_COLORS.lightGreen;
  if (errorRate < 0.10) return HEALTH_COLORS.yellow;
  if (errorRate < 0.25) return HEALTH_COLORS.orange;
  if (errorRate < 0.50) return HEALTH_COLORS.deepOrange;
  return HEALTH_COLORS.red;
};

const getHealthLabel = (successCount, errorCount, t) => {
  const total = successCount + errorCount;
  if (total === 0) return t('无调用数据');
  if (total < 3 && errorCount > 0) return t('轻微异常');
  const errorRate = errorCount / total;
  if (errorRate === 0) return t('运行正常');
  if (errorRate < 0.10) return t('轻微异常');
  if (errorRate < 0.25) return t('服务降级');
  if (errorRate < 0.50) return t('严重降级');
  return t('服务不可用');
};

const formatTime = (timestamp) => {
  const date = new Date(timestamp * 1000);
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  return `${month}-${day} ${hours}:${minutes}`;
};

const TOOLTIP_STYLE = {
  position: 'fixed',
  zIndex: 1070,
  padding: '8px 12px',
  borderRadius: 12,
  backgroundColor: 'rgba(28, 31, 35, 0.96)',
  color: '#fff',
  fontSize: 12,
  lineHeight: 1.6,
  pointerEvents: 'none',
  whiteSpace: 'nowrap',
  transform: 'translate(-50%, -100%)',
};

const CellTooltip = ({ cell, bucketSize, pos, t }) => {
  if (!pos) return null;

  const style = {
    ...TOOLTIP_STYLE,
    left: pos.x,
    top: pos.y - 6,
  };

  const content = cell ? (
    <>
      <div style={{ opacity: 0.65, marginBottom: 2 }}>
        {formatTime(cell.t)} ~ {formatTime(cell.t + bucketSize)}
      </div>
      <div>
        {t('成功')}: {cell.s} &nbsp; {t('失败')}: {cell.e}
      </div>
      <div>
        {t('成功率')}:{' '}
        {((cell.s / (cell.s + cell.e)) * 100).toFixed(1)}%
        <span
          style={{
            marginLeft: 6,
            color: getHealthColor(cell.s, cell.e),
            fontWeight: 600,
          }}
        >
          {getHealthLabel(cell.s, cell.e, t)}
        </span>
      </div>
    </>
  ) : (
    <span style={{ opacity: 0.7 }}>{t('无调用数据')}</span>
  );

  return ReactDOM.createPortal(<div style={style}>{content}</div>, document.body);
};

const TimelineBar = ({ cells, bucketSize, cellHeight, t }) => {
  const [hover, setHover] = useState(null); // { idx, x, y }
  const barRef = useRef(null);

  const handleMouseMove = useCallback(
    (e) => {
      const bar = barRef.current;
      if (!bar) return;
      const rect = bar.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const idx = Math.floor((x / rect.width) * cells.length);
      const clamped = Math.max(0, Math.min(cells.length - 1, idx));

      // Position tooltip above the hovered cell center
      const cellWidth = rect.width / cells.length;
      const cellCenterX = rect.left + clamped * cellWidth + cellWidth / 2;
      const cellTopY = rect.top;

      setHover({ idx: clamped, x: cellCenterX, y: cellTopY });
    },
    [cells.length],
  );

  const handleMouseLeave = useCallback(() => {
    setHover(null);
  }, []);

  const hoverIdx = hover ? hover.idx : -1;

  return (
    <>
      <div
        ref={barRef}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          width: '100%',
          cursor: 'pointer',
        }}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
      >
        {cells.map((cell, idx) => (
          <div
            key={idx}
            style={{
              flex: 1,
              minWidth: 0,
              height: cellHeight,
              backgroundColor: cell
                ? getHealthColor(cell.s, cell.e)
                : HEALTH_COLORS.noData,
              borderRadius: 2,
              opacity: hoverIdx === idx ? 0.7 : 1,
              transition: 'opacity 0.1s',
            }}
          />
        ))}
      </div>
      <CellTooltip
        cell={hover ? cells[hover.idx] : null}
        bucketSize={bucketSize}
        pos={hover}
        t={t}
      />
    </>
  );
};

const Legend = ({ t }) => {
  const items = [
    { color: HEALTH_COLORS.noData, label: t('无调用数据') },
    { color: HEALTH_COLORS.healthy, label: t('运行正常') },
    { color: HEALTH_COLORS.lightGreen, label: '< 2%' },
    { color: HEALTH_COLORS.yellow, label: '2~10%' },
    { color: HEALTH_COLORS.orange, label: '10~25%' },
    { color: HEALTH_COLORS.deepOrange, label: '25~50%' },
    { color: HEALTH_COLORS.red, label: '> 50%' },
  ];

  return (
    <div
      style={{
        display: 'flex',
        gap: 8,
        flexWrap: 'wrap',
        marginTop: 8,
      }}
    >
      {items.map((item) => (
        <div
          key={item.label}
          style={{ display: 'flex', alignItems: 'center', gap: 3 }}
        >
          <div
            style={{
              width: 8,
              height: 8,
              borderRadius: 2,
              backgroundColor: item.color,
            }}
          />
          <span style={{ fontSize: 10, color: 'var(--semi-color-text-2)' }}>
            {item.label}
          </span>
        </div>
      ))}
    </div>
  );
};

const buildCellArray = (healthData, groupCells) => {
  const cellMap = {};
  if (groupCells) {
    groupCells.forEach((cell) => {
      cellMap[cell.t] = cell;
    });
  }
  const result = [];
  for (let i = 0; i < healthData.bucket_count; i++) {
    const ts = healthData.start_time + i * healthData.bucket_size;
    result.push(cellMap[ts] || null);
  }
  return result;
};

const EMPTY_GROUPS = [];

const ModelHealthTimeline = ({
  healthData,
  modelName,
  groups,
  compact = false,
  showLegend = true,
  t,
}) => {
  const stableGroups = groups || EMPTY_GROUPS;

  const groupTimelines = useMemo(() => {
    if (!healthData || !healthData.data || stableGroups.length === 0)
      return null;

    const modelData = healthData.data[modelName] || {};

    return stableGroups.map((group) => ({
      group,
      cells: buildCellArray(healthData, modelData[group]),
    }));
  }, [healthData, modelName, stableGroups]);

  if (!healthData || !groupTimelines || groupTimelines.length === 0) {
    return null;
  }

  const cellHeight = compact ? 14 : 18;

  return (
    <div style={{ width: '100%' }}>
      {groupTimelines.map(({ group, cells }) => (
        <div key={group} style={{ marginBottom: compact ? 4 : 8 }}>
          <div
            style={{
              fontSize: compact ? 10 : 11,
              color: 'var(--semi-color-text-2)',
              marginBottom: 3,
              fontWeight: 500,
            }}
          >
            {group}
          </div>
          <TimelineBar
            cells={cells}
            bucketSize={healthData.bucket_size}
            cellHeight={cellHeight}
            t={t}
          />
        </div>
      ))}
      {showLegend && !compact && <Legend t={t} />}
    </div>
  );
};

export default ModelHealthTimeline;
