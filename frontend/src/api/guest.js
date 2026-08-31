import { request } from './request'

export function createGuestApi(requester = request) {
  return {
    getDeployments() {
      return requester('/api/guest/deployments')
    },
  }
}
