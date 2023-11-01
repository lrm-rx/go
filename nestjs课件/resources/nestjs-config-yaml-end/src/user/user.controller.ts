import { Controller, Get, Post } from '@nestjs/common';
import { UserService } from './user.service';
import { ConfigService } from '@nestjs/config';
// import { ConfgEnum } from 'src/enum/config.enum';

@Controller('user')
export class UserController {
  constructor(
    private userService: UserService,
    private configService: ConfigService,
  ) {}

  @Get()
  getUsers(): any {
    // const db = this.configService.get(ConfgEnum.DB);
    // const host = this.configService.get(ConfgEnum.DB_HOST);
    // console.log(
    //   '🚀 ~ file: user.controller.ts ~ line 15 ~ UserController ~ getUsers ~ db',
    //   db,
    //   host,
    // );
    const data = this.configService.get('db');
    console.log(
      '🚀 ~ file: user.controller.ts ~ line 23 ~ UserController ~ getUsers ~ data',
      data,
    );
    return this.userService.getUsers();
  }

  @Post()
  addUser(): any {
    return this.userService.addUser();
  }
}
