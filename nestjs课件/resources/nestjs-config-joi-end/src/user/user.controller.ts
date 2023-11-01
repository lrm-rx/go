import { Controller, Get, Post } from '@nestjs/common';
import { UserService } from './user.service';
import { ConfigService } from '@nestjs/config';
import { ConfgEnum } from 'src/enum/config.enum';

@Controller('user')
export class UserController {
  constructor(
    private userService: UserService,
    private configService: ConfigService,
  ) {}

  @Get()
  getUsers(): any {
    const db = this.configService.get(ConfgEnum.DB);
    const host = this.configService.get(ConfgEnum.DB_HOST);
    console.log(
      '🚀 ~ file: user.controller.ts ~ line 15 ~ UserController ~ getUsers ~ db',
      db,
      host,
    );
    const url = this.configService.get('DB_URL');
    console.log(
      '🚀 ~ file: user.controller.ts ~ line 23 ~ UserController ~ getUsers ~ url',
      url,
    );
    const port = this.configService.get('DB_PORT');
    console.log(
      '🚀 ~ file: user.controller.ts ~ line 28 ~ UserController ~ getUsers ~ port',
      port,
    );
    return this.userService.getUsers();
  }

  @Post()
  addUser(): any {
    return this.userService.addUser();
  }
}
