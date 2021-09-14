import { Inject, Service } from 'typedi'
import { SessionStore } from '~/stores/session.store'
import { SessionId, Session } from '~/types'

const SESSION_EXPIRY_TIME = 5 * 60 * 1000 // 5 mins
const CACHE_SESSIONS = false

@Service()
export class SessionService {
    public sessions: Map<SessionId, Session> = new Map()

    @Inject()
    private store: SessionStore

    async fetch(id: SessionId): Promise<Session> {
        const session: Session = await this.store.get(id)

        if (CACHE_SESSIONS) {
            this.sessions.set(id, session)
        }

        return session
    }

    async get(id: SessionId): Promise<Session> {
        let session: Session = this.sessions.get(id)

        if (!session) {
            if (session = await this.store.get(id)) {
                this.sessions.set(id, session)
            }
        }

        return session
    }

    set(id: SessionId, session: Session): Promise<void> {
        return this.store.set(id, session)
    }

    save(session: Session): Promise<void> {
        return this.set(session.id, session)
    }

    expire(id: SessionId): Promise<void> {
        return this.store.expire(id, SESSION_EXPIRY_TIME)
    }

    persist(id: SessionId): Promise<boolean> {
        return this.store.persist(id)
    }
}
