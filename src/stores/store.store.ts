import { ScanStreamOption } from 'ioredis'
import { Readable } from 'stream'
import { Inject, Service } from 'typedi'
import { StoreService } from '~/services/store.service'

export type Flags = (string | number)[]

export interface IStore {
    get<T>(key: string, flags: Flags): Promise<T>
    set<T>(key: string, data: T, flags: Flags): Promise<void>
    del<T>(key: string, flags: Flags): Promise<void>
}

@Service()
export abstract class Store implements IStore {
    public abstract readonly name: string
    
    @Inject()
    private storeService: StoreService

    private serializeKey(parts: Flags): string {
        return [this.name, ...parts].join('.')
    }

    private processData<T = unknown>(data: string, defaultValue: T = null): T | null {
        let res: T

        try {
            res = JSON.parse(data)
        } catch (err) {
            res = null
        }

        if (!res) {
            res = defaultValue
        }

        return res
    }

    public async get<T = unknown>(key: string, flags: Flags = [], defaultValue: T = null): Promise<T | null> {
        key = this.serializeKey([key, ...flags])

        const data = await this.storeService.store.get(key)

        return this.processData<T>(data, defaultValue)
    }

    public async set<T = unknown>(key: string, data: T, flags: Flags = []): Promise<void> {
        key = this.serializeKey([key, ...flags])

        await this.storeService.store.set(key, JSON.stringify(data))
    }

    public async del(key: string, flags: Flags = []): Promise<void> {
        key = this.serializeKey([key, ...flags])

        await this.storeService.store.del(key)
    }

    public async hget<T = unknown>(key: string, field: string, flags: Flags = [], defaultValue: T = null): Promise<T | null> {
        key = this.serializeKey([key, ...flags])

        const data = await this.storeService.store.hget(key, field)
        
        return this.processData<T>(data, defaultValue)
    }

    public async hset<T = unknown>(key: string, field: string, data: T, flags: Flags = []): Promise<void> {
        key = this.serializeKey([key, ...flags])

        await this.storeService.store.hset(key, field, JSON.stringify(data))
    }

    public async hdel(key: string, field: string, flags: Flags = []): Promise<void> {
        key = this.serializeKey([key, ...flags])

        await this.storeService.store.hdel(key, field)
    }

    public hscanStream(key: string, options: ScanStreamOption = {}, flags: Flags = []): Readable {
        key = this.serializeKey([key, ...flags])

        return this.storeService.store.hscanStream(key, options)
    }

    public hscan(key: string, options: ScanStreamOption = {}, flags: Flags = []): Promise<string[]> {
        return new Promise((resolve, reject) => {
            let keys: string[] = []
            
            const stream = this.hscanStream(key, options, flags)
            
            stream.on('data', (resKeys: string[]) => {
                keys = keys.concat(resKeys)
            })

            stream.on('error', reject)
            stream.on('end', () => resolve(keys))
        })
    }

    public async expire(key: string, seconds: number, flags: Flags = []): Promise<void> {
        key = this.serializeKey([key, ...flags])

        await this.storeService.store.expire(key, seconds)
    }

    public async persist(key: string, flags: Flags = []): Promise<boolean> {
        key = this.serializeKey([key, ...flags])
        const res = await this.storeService.store.persist(key)

        return Boolean(res)
    }
}
