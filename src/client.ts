import WebSocket from 'ws'
import { Packet, Session } from './types'
import { v4 } from 'uuid'
import { Permissions } from '@sakuraapp/common'
import { Room, User } from '@sakuraapp/shared'

export default class Client {
    public readonly socket: WebSocket
    public session: Session = {
        id: v4(),
    }

    public user: User

    constructor(socket: WebSocket) {
        this.socket = socket
    }

    send(data: Packet) {
        if (!data.t) {
            data.t = Date.now()
        }
        
        this.socket.send(JSON.stringify(data))
    }

    getPermissions(room: Room): Permissions[] {
        if (this.session.userId === room.owner._id.toHexString()) {
            return Object.values(Permissions) as Permissions[]
        }

        return [Permissions.QUEUE_ADD]
    }

    hasPermissions(perms: Permissions[], room: Room) {
        const ownPerms = this.getPermissions(room)

        return perms.every((perm) => ownPerms.includes(perm))
    }
}
