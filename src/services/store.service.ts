import Redis from 'ioredis'
import { Service } from 'typedi'

@Service()
export class StoreService {
    public readonly store = this.create()

    create(): Redis.Redis {
        return new Redis()
    }
}
