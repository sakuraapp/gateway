import { readFileSync } from 'fs'
import { join } from 'path'

if (!process.env.JWT_PUBLIC_KEY) {
    process.env.JWT_PUBLIC_KEY = readFileSync(
        join(__dirname, '/public.pem'),
        'utf8'
    )
}
