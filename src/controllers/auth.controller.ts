import { Inject, Service } from 'typedi'
import { Handler, NextFn } from 'ws-decorators'
import { Opcodes } from '@sakuraapp/common'
import { verify } from 'jsonwebtoken'
import { SessionId, TokenPayload } from '~/types'
import Client from '~/client'
import { logger } from '~/utils/logger'
import Database from '~/database'
import { User } from '@sakuraapp/shared'
import { SessionService } from '~/services/session.service'
import RoomController from './room.controller'
import { Dispatcher } from '~/dispatchers/dispatcher.dispatcher'

export function requireAuth(data: unknown, client: Client, next: NextFn) {
    if (client.session.userId) {
        next()
    }
}

@Service()
export default class AuthController {
    @Inject()
    private database: Database

    @Inject()
    private sessionService: SessionService

    @Inject()
    private dispatcher: Dispatcher

    @Inject()
    private roomController: RoomController

    authenticate(token: string): Promise<User> {
        return new Promise((resolve, reject) => {
            verify(token, process.env.JWT_PUBLIC_KEY, (err, payload: TokenPayload) => {
                if (err) {
                    reject(err)
                } else {
                    this.database.user.findOne(payload.id)
                        .then(resolve)
                        .catch(reject)
                }
            })
        })
    }

    @Handler(Opcodes.AUTHENTICATE)
    async handleAuth({
        token,
        sessionId
    }: {
        token: string,
        sessionId: SessionId
    }, client: Client) {
        try {
            const user = await this.authenticate(token)

            client.user = user
            client.session.userId = user.id

            let addSession = true

            if (sessionId) {
                const sess = await this.sessionService.get(sessionId)

                if (sess) {
                    if (sess.userId === user.id) {
                        client.session = sess
                        addSession = false

                        await this.sessionService.persist(sessionId)
                    } else {
                        logger.warn(`User ${user.id} is trying to hijack a session`, { session: sess })
                        
                        throw new Error('Session hijack attempted')
                    }
                }
            }

            if (addSession) {
                await this.sessionService.save(client.session)
            }

            this.dispatcher.register({ userId: user.id }) // no await

            client.send({
                op: Opcodes.AUTHENTICATE,
                d: { sessionId: client.session.id },
            })

            if (client.session.roomId) {
                this.roomController.handleJoinRoom(client.session.roomId, client, 0, true)
            }
        } catch (err) {
            logger.debug(err)
            client.socket.close()
        }
    }

    @Handler(Opcodes.DISCONNECT)
    async handleDisconnect(data: unknown, client: Client) {
        await this.sessionService.expire(client.session.id)
        await this.sessionService.save(client.session)

        this.dispatcher.unregister({ userId: client.user.id })
    }
}