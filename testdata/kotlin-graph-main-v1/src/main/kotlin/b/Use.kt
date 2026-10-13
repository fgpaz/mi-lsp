package b

import a.Box
import a.decorate
import a.echo
import a.greeting
import fixture.JavaInput
import fixture.External

fun echo(value: Int): Int = value * 2
fun use(): String {
    val box = Box(3)
    val first = a.echo(1)
    val again = a.echo(2)
    val overload = a.echo("literal echo Box pick")
    val number = box.pick(first + again) + echo(4) + JavaInput.number() + External.number()
    return overload.decorate() + greeting + number
}
