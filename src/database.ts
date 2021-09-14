import { Inject, Service } from 'typedi'
import { StoreService } from '~/services/store.service'
import { logger } from '~/utils/logger'
import {
    MikroORM,
    RedisCacheAdapter,
    BaseRepository,
    Room,
    User,
    Credentials,
    Profile,
    createDatabase,
} from '@sakuraapp/shared'

const CACHE_EXPIRATION_TIME = 10 * 60 // 10 mins

@Service()
export default class Database {
    @Inject()
    private storeService: StoreService

    public orm: MikroORM
    public user: BaseRepository<User>
    public room: BaseRepository<Room>

    async connect(): Promise<void> {
        this.orm = await createDatabase({
            entities: [User, Credentials, Profile, Room],
            dbName: process.env.DB_NAME || 'sakura',
            host: process.env.DB_HOST || '127.0.0.1',
            port: Number(process.env.DB_PORT || '27017'),
            validate: true,
            resultCache: {
                adapter: RedisCacheAdapter,
                options: {
                    client: this.storeService.store,
                    expiration: CACHE_EXPIRATION_TIME,
                },
            },
        })

        logger.info('Connected to database')
    }

    async init(): Promise<void> {
        await this.connect()

        this.user = this.orm.em.getRepository(User)
        this.room = this.orm.em.getRepository(Room)
    }
}
