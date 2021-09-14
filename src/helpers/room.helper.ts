import { RoomInfo } from '@sakuraapp/common'
import { Room } from '@sakuraapp/shared'
import { UserHelper } from './user.helper'

export class RoomHelper {
    static build(room: Room): RoomInfo {
        return {
            id: room._id,
            name: room.name,
            owner: UserHelper.build(room.owner),
            private: room.private,
        }
    }
}
