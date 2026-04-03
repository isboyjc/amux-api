import React, { useEffect, useRef, useState, memo } from 'react';

const Digit = memo(({ value, height, delay = 0 }) => {
  const [current, setCurrent] = useState(0);

  useEffect(() => {
    // requestAnimationFrame ensures the browser has painted the initial state (0)
    // before we trigger the transition to the target value
    const raf = requestAnimationFrame(() => {
      const timer = setTimeout(() => setCurrent(value), delay);
      Digit._cleanup = () => clearTimeout(timer);
    });
    return () => {
      cancelAnimationFrame(raf);
      Digit._cleanup?.();
    };
  }, [value, delay]);

  return (
    <span
      style={{
        display: 'inline-block',
        height,
        overflow: 'hidden',
        width: '0.62em',
        textAlign: 'center',
      }}
    >
      <span
        style={{
          display: 'flex',
          flexDirection: 'column',
          transition: `transform 2s cubic-bezier(0.16, 1, 0.3, 1)`,
          transform: `translateY(${-current * height}px)`,
        }}
      >
        {Array.from({ length: 10 }, (_, i) => (
          <span
            key={i}
            style={{
              height,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {i}
          </span>
        ))}
      </span>
    </span>
  );
});

Digit.displayName = 'Digit';

const Counter = memo(({ value, className, style }) => {
  const text = String(value);
  const ref = useRef(null);
  const [charHeight, setCharHeight] = useState(0);

  useEffect(() => {
    if (ref.current) {
      setCharHeight(ref.current.offsetHeight);
    }
  }, [className, style]);

  const digits = text.split('');
  const digitCount = digits.filter((c) => c >= '0' && c <= '9').length;
  let digitIndex = 0;

  return (
    <span
      className={className}
      style={{
        ...style,
        display: 'inline-flex',
        alignItems: 'center',
        lineHeight: 1,
      }}
    >
      <span
        ref={ref}
        style={{
          position: 'absolute',
          visibility: 'hidden',
          pointerEvents: 'none',
        }}
        aria-hidden
      >
        0
      </span>
      {charHeight > 0 &&
        digits.map((char, i) => {
          if (char >= '0' && char <= '9') {
            const delay = 100 + (digitIndex / digitCount) * 400;
            digitIndex++;
            return (
              <Digit
                key={i}
                value={parseInt(char, 10)}
                height={charHeight}
                delay={delay}
              />
            );
          }
          return (
            <span key={i} style={{ display: 'inline-block' }}>
              {char}
            </span>
          );
        })}
    </span>
  );
});

Counter.displayName = 'Counter';

export default Counter;
