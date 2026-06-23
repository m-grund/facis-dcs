import http from '@/api/http'

interface DIDDocument {
    id: string
}

export async function getLocalDIDFile(): Promise<DIDDocument> {
    return http
        .get('/.well-known/did.json')
        .then(res => res.data)
}