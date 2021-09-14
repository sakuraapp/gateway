import { Opcodes } from '@sakuraapp/common'
import { Room, User } from '@sakuraapp/shared'
import { Inject, Service } from 'typedi'
import Database from '~/database'
import { RoomStore } from '~/stores/room.store'
import { GatewayMessage, RoomId, UserId } from '~/types'
import { BrokerService } from './broker.service'

interface AddUserEvent {
    user: {
        id: string
    }
}

@Service()
export class RoomService {
    public users: Map<RoomId, UserId[]> = new Map()

    @Inject()
    private database: Database

    @Inject()
    private store: RoomStore

    @Inject()
    brokerService: BrokerService

    init() {
        this.brokerService.subscribe(Opcodes.ADD_USER, (message: GatewayMessage<AddUserEvent>) => {
            const roomId = message.target.roomId
            const userId = message.message.d.user.id

            const ids = this.users.get(roomId)

            if (ids) {
                ids.push(userId)

                console.log(this.users.get(roomId))
            }
        })
    }

    private async fetchUserIds(roomId: RoomId): Promise<UserId[]> {
        const ids = await this.store.hscan('sessions', {}, [roomId])

        this.users.set(roomId, ids)

        return ids
    }

    public async getUserIds(roomId: RoomId): Promise<UserId[]> {
        const ids = this.users.get(roomId)
        
        if (ids) {
            return ids
        } else {
            return await this.fetchUserIds(roomId)
        }
    }

    public async getUsers(room: Room, except: UserId[] = []): Promise<User[]> {
        let ids = await this.getUserIds(room._id)

        if (except.length > 0) {
            ids = ids.filter((id) => !except.includes(id))
        }

        const users = await this.database.user.find({
            $in: ids
        })

        return users
    }
}
