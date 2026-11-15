import { apiClient } from './api'
import { demoClient } from './demo'

export const isDemo = (import.meta.env.VITE_DATA_SOURCE || 'demo') === 'demo'
export const client = isDemo ? demoClient : apiClient
