import { Service } from 'typedi'
import Client from '~/client'
import { RoomId, SessionId, UserId } from '~/types'

@Service()
export class SessionManager {
    private readonly clients: Client[] = []

    add(client: Client): void {
        this.clients.push(client)
    }

    remove(client: Client): void {
        const idx = this.clients.indexOf(client)

        if (idx > -1) {
            this.clients.splice(idx, 1)
        }
    }

    get(sessionId: SessionId): Client | null {
        return this.clients.find((client) => client.session.id === sessionId)
    }

    getAll(): Client[] {
        return this.clients
    }

    getAllByRoomId(roomId: RoomId): Client[] {
        return this.clients.filter((client) => client.session.roomId === roomId)
    }

    getAllByUserId(userId: UserId): Client[] {
        return this.clients.filter((client) => client.session.userId === userId)
    }

    getInIds(ids: SessionId[]): Client[] {
        return this.clients.filter((client) =>
            ids.includes(client.session.id))
    }

    getInUserIds(userIds: UserId[]): Client[] {
        return this.clients.filter((client) =>
            userIds.includes(client.session.userId))
    }
}
