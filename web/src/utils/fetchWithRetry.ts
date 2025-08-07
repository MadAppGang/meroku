interface RetryConfig {
  maxRetries?: number;
  retryDelay?: number;
  retryCondition?: (response: Response) => boolean;
}

const DEFAULT_CONFIG: Required<RetryConfig> = {
  maxRetries: 3,
  retryDelay: 1000,
  retryCondition: (response: Response) => {
    // Retry on server errors (5xx) and specific auth errors
    return response.status >= 500 || response.status === 401 || response.status === 403;
  }
};

/**
 * Fetch wrapper with retry logic to prevent infinite retry loops
 */
export async function fetchWithRetry(
  url: string,
  options?: RequestInit,
  config: RetryConfig = {}
): Promise<Response> {
  const { maxRetries, retryDelay, retryCondition } = { ...DEFAULT_CONFIG, ...config };
  
  let lastError: Error | null = null;
  
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(url, options);
      
      // If response is ok or we shouldn't retry, return it
      if (response.ok || !retryCondition(response)) {
        return response;
      }
      
      // If this is the last attempt, return the response anyway
      if (attempt === maxRetries) {
        return response;
      }
      
      // Wait before retrying
      await delay(retryDelay * Math.pow(2, attempt)); // Exponential backoff
      
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));
      
      // If this is the last attempt, throw the error
      if (attempt === maxRetries) {
        throw lastError;
      }
      
      // Wait before retrying
      await delay(retryDelay * Math.pow(2, attempt));
    }
  }
  
  // This should never be reached, but just in case
  throw lastError || new Error('Max retries exceeded');
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Enhanced fetch wrapper that handles token signature errors specifically
 */
export async function fetchWithTokenRetry(
  url: string,
  options?: RequestInit
): Promise<Response> {
  return fetchWithRetry(url, options, {
    maxRetries: 3,
    retryDelay: 1000,
    retryCondition: (response: Response) => {
      // Only retry on specific auth errors and server errors
      // Don't retry on token signature errors after 3 attempts
      return response.status >= 500;
    }
  });
}