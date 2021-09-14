import { Opcodes } from '@sakuraapp/common'
import { Inject, Service } from 'typedi'
import { Handler } from 'ws-decorators'
import Client from '~/client'
import Database from '~/database'
import { Dispatcher } from '~/dispatchers/dispatcher.dispatcher'
import { RoomHelper } from '~/helpers/room.helper'
import { UserHelper } from '~/helpers/user.helper'
import { RoomService } from '~/services/room.service'
import { SessionService } from '~/services/session.service'
import { RoomStore } from '~/stores/room.store'
import { RoomId, SessionId } from '~/types'

@Service()
export default class RoomController {
    @Inject()
    private database: Database

    @Inject()
    private store: RoomStore

    @Inject()
    private dispatcher: Dispatcher

    @Inject()
    private roomService: RoomService

    @Inject()
    private sessionService: SessionService

    private async removeClient(client: Client): Promise<void> {
        const { session } = client
        const { id, roomId, userId } = session

        if (roomId) {
            const sessions: SessionId[] = await this.store.hget('sessions', userId, [roomId], [])
            const idx = sessions.indexOf(id)

            client.session.roomId = null

            if (idx > -1) {
                sessions.splice(idx, 1)

                if (sessions.length > 0) {
                    await this.store.hset('sessions', userId, [roomId])
                } else {
                    await this.store.hdel('sessions', userId, [roomId])
                    await this.dispatcher.dispatch(
                        Opcodes.REMOVE_USER,
                        client.session.userId,
                        { roomId },
                    )
                }
            }
        }
    }

    @Handler(Opcodes.LEAVE_ROOM)
    @Handler(Opcodes.DISCONNECT)
    handleLeaveRoom(data: unknown, client: Client): Promise<void> {
        return this.removeClient(client)
    }

    @Handler(Opcodes.JOIN_ROOM)
    async handleJoinRoom(roomId: RoomId, client: Client, t: number, force = false): Promise<void> {
        const { session } = client
        const { userId } = session

        if (session.roomId) {
            if (session.roomId !== roomId) {
                await this.removeClient(client)
            } else if (!force) {
                return client.send({
                    op: Opcodes.JOIN_ROOM,
                    d: roomId,
                })
            }
        }

        const room = await this.database.room.findOne(roomId)

        if (!room) {
            return client.send({
                op: Opcodes.JOIN_ROOM,
                d: { status: 404 },
            })
        }

        const ownerId = room.owner.id

        if (!force && room.private && ownerId !== userId) {
            const invite = await this.store.hget('invites', userId)

            if (!invite) {
                await this.dispatcher.dispatch(
                    Opcodes.ROOM_JOIN_REQUEST,
                    userId,
                    {
                        userId: ownerId,
                    }
                )

                return
            } else {
                this.store.hdel('invites', userId) // no await
            }
        }

        const sessions: SessionId[] = await this.store.hget('sessions', userId, [roomId], [])

        if (!sessions.includes(session.id)) {
            sessions.push(session.id)

            await this.store.hset('sessions', userId, sessions, [roomId])
        }

        await this.dispatcher.dispatch(
            Opcodes.ADD_USER,
            {
                user: UserHelper.build(client.user),
            },
            {
                roomId,
            }
        )

        const users = await this.roomService.getUsers(room, [userId])

        users.push(client.user)
    
        client.session.roomId = room._id
        client.send({
            op: Opcodes.JOIN_ROOM,
            d: {
                status: 200,
                room: RoomHelper.build(room),
                users: users.map((user) => UserHelper.build(user)),
                permissions: client.getPermissions(room)
            },
        })

        await this.sessionService.save(client.session)
    }
}
