import { useState, useEffect, useCallback, useRef } from 'react';

interface CacheEntry<T> {
  data: T;
  timestamp: number;
  expiry: number;
}

interface CacheOptions {
  ttl?: number; // 生存时间，毫秒
  maxSize?: number; // 最大缓存条目数
  staleWhileRevalidate?: number; // 过期后仍可使用的时间，毫秒
}

interface SmartCacheResult<T> {
  data: T | null;
  loading: boolean;
  error: Error | null;
  isStale: boolean;
  refresh: () => Promise<void>;
  clear: () => void;
}

class SmartCache {
  private cache = new Map<string, CacheEntry<unknown>>();
  private maxSize: number;
  
  constructor(maxSize: number = 100) {
    this.maxSize = maxSize;
  }

  get<T>(key: string): CacheEntry<T> | null {
    const entry = this.cache.get(key);
    if (!entry) return null;
    
    // 检查是否过期
    if (Date.now() > entry.expiry) {
      this.cache.delete(key);
      return null;
    }
    
    return entry as CacheEntry<T>;
  }

  set<T>(key: string, data: T, ttl: number): void {
    // 如果缓存已满，删除最老的条目
    if (this.cache.size >= this.maxSize) {
      const oldestKey = this.cache.keys().next().value as string;
      this.cache.delete(oldestKey);
    }

    const entry: CacheEntry<T> = {
      data,
      timestamp: Date.now(),
      expiry: Date.now() + ttl
    };

    this.cache.set(key, entry);
  }

  isStale(key: string, staleWhileRevalidate?: number): boolean {
    const entry = this.cache.get(key);
    if (!entry || !staleWhileRevalidate) return false;
    
    const staleTime = entry.timestamp + staleWhileRevalidate;
    return Date.now() > staleTime;
  }

  delete(key: string): void {
    this.cache.delete(key);
  }

  clear(): void {
    this.cache.clear();
  }

  size(): number {
    return this.cache.size;
  }

  keys(): string[] {
    return Array.from(this.cache.keys());
  }
}

// 全局缓存实例
const globalCache = new SmartCache(200);

export function useSmartCache<T>(
  key: string,
  fetchFn: () => Promise<T>,
  options: CacheOptions = {}
): SmartCacheResult<T> {
  const {
    ttl = 5 * 60 * 1000, // 默认5分钟
    staleWhileRevalidate = 30 * 1000, // 默认30秒
  } = options;

  // 检查缓存中是否有初始数据
  const cachedEntry = globalCache.get<T>(key);
  const [data, setData] = useState<T | null>(cachedEntry?.data || null);
  const [loading, setLoading] = useState(!cachedEntry); // 如果没有缓存数据，则显示加载状态
  const [error, setError] = useState<Error | null>(null);
  const [isStale, setIsStale] = useState(false);
  
  const abortControllerRef = useRef<AbortController | null>(null);
  const isMountedRef = useRef(true);
  const fetchFnRef = useRef(fetchFn);
  
  // 更新fetchFn引用
  useEffect(() => {
    fetchFnRef.current = fetchFn;
  }, [fetchFn]);

  // 检查缓存并获取数据
  const loadData = useCallback(async (forceRefresh = false) => {
    console.log(`🔄 [useSmartCache] loadData开始 - key: ${key}, forceRefresh: ${forceRefresh}`);
    
    // 取消之前的请求
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    if (!forceRefresh) {
      // 检查缓存
      const cachedEntry = globalCache.get<T>(key);
      console.log(`💾 [useSmartCache] 缓存检查 - key: ${key}, 有缓存: ${!!cachedEntry}`);
      
      if (cachedEntry) {
        if (isMountedRef.current) {
          console.log(`✅ [useSmartCache] 使用缓存数据 - key: ${key}`);
          setData(cachedEntry.data);
          setError(null);
          setIsStale(globalCache.isStale(key, staleWhileRevalidate));
          setLoading(false); // 确保设置loading为false
        }
        
        // 如果数据不新鲜，在后台更新
        if (globalCache.isStale(key, staleWhileRevalidate)) {
          console.log(`🔄 [useSmartCache] 数据过期，后台更新 - key: ${key}`);
          // 后台更新，不显示loading
          setTimeout(() => loadData(true), 0);
        }
        return;
      }
    }

    if (isMountedRef.current) {
      console.log(`⏳ [useSmartCache] 开始加载数据 - key: ${key}`);
      setLoading(true);
      setError(null);
    }

    try {
      abortControllerRef.current = new AbortController();
      
      console.log(`📡 [useSmartCache] 调用fetchFn - key: ${key}`);
      const result = await fetchFnRef.current();
      console.log(`✅ [useSmartCache] fetchFn成功 - key: ${key}`, result);
      console.log(`🔍 [useSmartCache] isMountedRef.current: ${isMountedRef.current} - key: ${key}`);
      
      // 在React StrictMode中，强制设置组件为已挂载状态
      // 因为API请求成功意味着组件仍然需要这些数据
      if (!isMountedRef.current) {
        console.log(`🔧 [useSmartCache] 重新设置isMountedRef为true - key: ${key}`);
        isMountedRef.current = true;
      }
      
      if (isMountedRef.current) {
        console.log(`💾 [useSmartCache] 设置数据和缓存 - key: ${key}`);
        setData(result);
        setError(null);
        setIsStale(false);
        setLoading(false); // 明确设置loading为false
        
        // 缓存数据
        globalCache.set(key, result, ttl);
      } else {
        console.warn(`⚠️ [useSmartCache] 组件已卸载，跳过设置数据 - key: ${key}`);
      }
    } catch (err) {
      console.error(`❌ [useSmartCache] 加载失败 - key: ${key}`, err);
      if (isMountedRef.current && err instanceof Error && err.name !== 'AbortError') {
        console.error('useSmartCache error:', err);
        setError(err);
        
        // 如果是认证错误，清除缓存并重定向到登录页
        if (err.message.includes('401') || err.message.includes('unauthorized') || err.message.includes('未提供认证令牌')) {
          console.warn('认证错误，清除缓存');
          globalCache.delete(key);
          // 可以在这里添加重定向到登录页的逻辑
          // window.location.href = '/login';
        }
        
        // 如果有缓存数据，继续使用但标记为过期
        const cachedEntry = globalCache.get<T>(key);
        if (cachedEntry) {
          setData(cachedEntry.data);
          setIsStale(true);
        }
        
        setLoading(false); // 即使出错也要设置loading为false
      }
    } finally {
      if (isMountedRef.current) {
        console.log(`🏁 [useSmartCache] 完成加载 - key: ${key}`);
        setLoading(false);
      }
    }
  }, [key, ttl, staleWhileRevalidate]); // 移除fetchFn依赖，使用ref

  // 刷新数据
  const refresh = useCallback(async () => {
    await loadData(true);
  }, [loadData]);

  // 清除缓存
  const clear = useCallback(() => {
    globalCache.delete(key);
    if (isMountedRef.current) {
      setData(null);
      setError(null);
      setIsStale(false);
    }
  }, [key]);

  // 初始加载
  useEffect(() => {
    console.log(`🚀 [useSmartCache] 初始加载 effect 触发 for key: ${key}`);
    loadData();
  }, [loadData, key]);

  // 清理
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  return {
    data,
    loading,
    error,
    isStale,
    refresh,
    clear,
  };
}

// 导出缓存实例用于调试
export { globalCache };