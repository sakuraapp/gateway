import { Opcodes } from '@sakuraapp/common'

export type SessionId = string
export type UserId = string
export type RoomId = string

export interface Session {
    id: SessionId
    userId?: UserId
    roomId?: RoomId
}

export interface MessageTarget {
    userId?: UserId
    roomId?: RoomId
    ignored?: SessionId[]
}

export interface Packet<T = unknown> {
    op: Opcodes
    d?: T
    t?: number
}

export interface TokenPayload {
    id: string
}
