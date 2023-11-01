import {
  Controller,
  Delete,
  Get,
  Patch,
  Post,
  Inject,
  LoggerService,
  Body,
  Param,
  Query,
  UseFilters,
  UnauthorizedException,
  ParseIntPipe,
  UseGuards,
  Req,
} from '@nestjs/common';
import { UserService } from './user.service';
import { ConfigService } from '@nestjs/config';
import { User } from './user.entity';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { getUserDto } from './dto/get-user.dto';
import { TypeormFilter } from 'src/filters/typeorm.filter';
import { CreateUserPipe } from './pipes/create-user.pipe';
import { CreateUserDto } from './dto/create-user.dto';
// import { AuthGuard } from '@nestjs/passport';
import { AdminGuard } from 'src/guards/admin.guard';
import { JwtGuard } from 'src/guards/jwt.guard';
import { Serialize } from 'src/decorators/serialize.decorator';
import { PublicUserDto } from './dto/public-user.dto';
@Controller('user')
@UseFilters(new TypeormFilter())
// @UseGuards(AuthGuard('jwt'))
@UseGuards(JwtGuard)
export class UserController {
  // private logger = new Logger(UserController.name);

  constructor(
    private userService: UserService,
    private configService: ConfigService,
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
  ) {
    // this.logger.log('UserController init');
  }

  @Get('/profile')
  // @UseGuards(AuthGuard('jwt'))
  getUserProfile(
    @Query('id', ParseIntPipe) id: any,
    // 这里req中的user是通过AuthGuard('jwt')中的validate方法返回的
    // PassportModule来添加的
    // @Req() req
  ): any {
    // console.log(
    //   '🚀 ~ file: auth.controller.ts ~ line 34 ~ AuthController ~ signup ~ req',
    //   req.user,
    // );
    // 这是不标准的使用方法
    return this.userService.findProfile(id);
  }

  // todo
  // logs Modules
  @Get('/logs')
  getUserLogs(): any {
    return this.userService.findUserLogs(2);
  }

  @Get('/logsByGroup')
  async getLogsByGroup(): Promise<any> {
    const res = await this.userService.findLogsByGroup(2);
    // return res.map((o) => ({
    //   result: o.result,
    //   count: o.count,
    // }));
    return res;
  }

  @Get()
  // 非常重要的知识点
  // 1. 装饰器的执行顺序，方法的装饰器如果有多个，则是从下往上执行
  // @UseGuards(AdminGuard)
  // @UseGuards(AuthGuard('jwt'))
  // 2. 如果使用UseGuard传递多个守卫，则从前往后执行，如果前面的Guard没有通过，则后面的Guard不会执行
  @UseGuards(AdminGuard)
  @Serialize(PublicUserDto)
  getUsers(@Query() query: getUserDto): any {
    // page - 页码，limit - 每页条数，condition-查询条件(username, role, gender)，sort-排序
    // 前端传递的Query参数全是string类型，需要转换成number类型
    // this.logger.log(`请求getUsers成功`);
    // this.logger.warn(`请求getUsers成功`);
    // this.logger.error(`请求getUsers成功`);
    return this.userService.findAll(query);
    // return this.userService.getUsers();
  }

  @Post()
  addUser(@Body(CreateUserPipe) dto: CreateUserDto): any {
    const user = dto as User;
    // user -> dto.username
    // return this.userService.addUser();
    return this.userService.create(user);
  }

  @Get('/:id')
  getUser(): any {
    return 'hello world';
    // return this.userService.getUsers();
  }

  @Patch('/:id')
  updateUser(
    @Body() dto: any,
    @Param('id', ParseIntPipe) id: number,
    @Req() req,
    // @Headers('Authorization') headers: any,
  ): any {
    // console.log(
    //   '🚀 ~ file: user.controller.ts ~ line 76 ~ UserController ~ headers',
    //   headers,
    // );
    if (id === parseInt(req.user?.userId)) {
      console.log(123);
      // 说明是同一个用户在修改
      // todo
      // 权限1：判断用户是否是自己
      // 权限2：判断用户是否有更新user的权限
      // 返回数据：不能包含敏感的password等信息
      const user = dto as User;
      return this.userService.update(id, user);
    } else {
      throw new UnauthorizedException();
    }
  }

  // 1.controller名 vs service名 vs repository名应该怎么取
  // 2.typeorm里面delete与remove的区别
  @Delete('/:id') // RESTfull Method
  removeUser(@Param('id') id: number): any {
    // 权限：判断用户是否有更新user的权限
    return this.userService.remove(id);
  }
}
