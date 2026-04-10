import { useState, useEffect, useRef, useCallback } from 'react';
import { API } from '../../helpers';

export const useModelHealthData = () => {
  const [healthData, setHealthData] = useState(null);
  const [healthLoading, setHealthLoading] = useState(false);
  const [timeRange, setTimeRange] = useState('24h');
  const requestIdRef = useRef(0);

  const loadHealth = useCallback(async (range) => {
    const id = ++requestIdRef.current;
    setHealthLoading(true);
    try {
      const res = await API.get(`/api/pricing/health?range=${range}`);
      // Only update state if this is still the latest request
      if (id === requestIdRef.current && res.data.success) {
        setHealthData(res.data.data);
      }
    } catch {
      // Health data is non-critical, silent fail
    }
    if (id === requestIdRef.current) {
      setHealthLoading(false);
    }
  }, []);

  useEffect(() => {
    loadHealth(timeRange);
  }, [timeRange, loadHealth]);

  return {
    healthData,
    healthLoading,
    timeRange,
    setTimeRange,
  };
};
