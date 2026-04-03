import { useCallback, useEffect, useMemo, useRef, useState, memo } from 'react';
import './LogoLoop.css';

const ANIMATION_CONFIG = { SMOOTH_TAU: 0.25, MIN_COPIES: 2, COPY_HEADROOM: 2 };

const LogoLoop = memo(
  ({
    items,
    speed = 80,
    direction = 'left',
    logoHeight = 28,
    gap = 32,
    pauseOnHover = true,
    fadeOut = true,
    className,
    style,
  }) => {
    const containerRef = useRef(null);
    const trackRef = useRef(null);
    const seqRef = useRef(null);
    const rafRef = useRef(null);
    const lastTsRef = useRef(null);
    const offsetRef = useRef(0);
    const velocityRef = useRef(0);

    const [seqWidth, setSeqWidth] = useState(0);
    const [copyCount, setCopyCount] = useState(ANIMATION_CONFIG.MIN_COPIES);
    const [isHovered, setIsHovered] = useState(false);

    const targetVelocity = useMemo(() => {
      const mag = Math.abs(speed);
      return direction === 'left' ? mag : -mag;
    }, [speed, direction]);

    const updateDimensions = useCallback(() => {
      const cw = containerRef.current?.clientWidth ?? 0;
      const sw = seqRef.current?.getBoundingClientRect()?.width ?? 0;
      if (sw > 0) {
        setSeqWidth(Math.ceil(sw));
        setCopyCount(
          Math.max(
            ANIMATION_CONFIG.MIN_COPIES,
            Math.ceil(cw / sw) + ANIMATION_CONFIG.COPY_HEADROOM,
          ),
        );
      }
    }, []);

    useEffect(() => {
      if (!containerRef.current || !seqRef.current) return;
      const ro = new ResizeObserver(updateDimensions);
      ro.observe(containerRef.current);
      ro.observe(seqRef.current);
      updateDimensions();
      return () => ro.disconnect();
    }, [updateDimensions, items, gap, logoHeight]);

    useEffect(() => {
      const track = trackRef.current;
      if (!track) return;

      const animate = (ts) => {
        if (lastTsRef.current === null) lastTsRef.current = ts;
        const dt = Math.max(0, ts - lastTsRef.current) / 1000;
        lastTsRef.current = ts;

        const target = isHovered && pauseOnHover ? 0 : targetVelocity;
        const ease = 1 - Math.exp(-dt / ANIMATION_CONFIG.SMOOTH_TAU);
        velocityRef.current += (target - velocityRef.current) * ease;

        if (seqWidth > 0) {
          let next = offsetRef.current + velocityRef.current * dt;
          next = ((next % seqWidth) + seqWidth) % seqWidth;
          offsetRef.current = next;
          track.style.transform = `translate3d(${-next}px, 0, 0)`;
        }

        rafRef.current = requestAnimationFrame(animate);
      };

      rafRef.current = requestAnimationFrame(animate);
      return () => {
        if (rafRef.current) cancelAnimationFrame(rafRef.current);
        lastTsRef.current = null;
      };
    }, [targetVelocity, seqWidth, isHovered, pauseOnHover]);

    const rootClass = [
      'logoloop',
      fadeOut && 'logoloop--fade',
      className,
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <div
        ref={containerRef}
        className={rootClass}
        style={{
          '--logoloop-gap': `${gap}px`,
          '--logoloop-logoHeight': `${logoHeight}px`,
          width: '100%',
          ...style,
        }}
      >
        <div
          className='logoloop__track'
          ref={trackRef}
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
        >
          {Array.from({ length: copyCount }, (_, i) => (
            <ul
              className='logoloop__list'
              key={i}
              ref={i === 0 ? seqRef : undefined}
              aria-hidden={i > 0}
            >
              {items.map((node, j) => (
                <li className='logoloop__item' key={`${i}-${j}`}>
                  <span className='logoloop__node'>{node}</span>
                </li>
              ))}
            </ul>
          ))}
        </div>
      </div>
    );
  },
);

LogoLoop.displayName = 'LogoLoop';

export default LogoLoop;
