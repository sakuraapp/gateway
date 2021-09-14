import http from 'http'
import https from 'https'

export function createServer(): http.Server {
    if (Number(process.env.USE_SSL)) {
        return https.createServer({
            cert: process.env.SSL_CERT,
            key: process.env.SSL_KEY,
        })
    } else {
        return http.createServer()
    }
}